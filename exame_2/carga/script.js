// Carga do experimento 2: laço aberto, duas classes de cliente, timeout e
// retry no cliente.
//
// "Aberto": a taxa de pedidos NOVOS não depende das respostas. O executor
// constant-arrival-rate inicia iterações a uma taxa fixa; cada iteração é um
// pedido lógico, que pode virar várias tentativas HTTP.
//
// Números comuns: o instante de chegada e a classe de cada pedido novo saem
// de um hash de (SEED, número global da iteração), e não de Math.random. Com
// a mesma SEED, as oito configurações de uma célula (4 políticas de fila × 2
// de cliente) veem exatamente a mesma sequência de pedidos novos. Só os
// retries diferem, porque dependem do servidor.
//
// Parâmetros (variáveis de ambiente, definidas pelo orquestrador):
//   TARGET            URL do endpoint /work
//   RATE              pedidos novos por segundo (aceita fração)
//   DURATION_S        duração da carga
//   POLICY            none | immediate | fixed | backoff
//   MAX_RETRIES       tentativas extras por pedido (ignorado em "none")
//   FIXED_DELAY_MS    espera antes de cada retry em "fixed"
//   BACKOFF_BASE_MS   base do backoff exponencial
//   BACKOFF_CAP_MS    teto do backoff
//   CLASSES           "nome:timeout_ms:fração,..." (padrão impatient:100:0.5,patient:200:0.5)
//   ARRIVAL_JITTER_MS espalhamento do instante de chegada
//   SEED              semente dos números comuns
//   VUS, MAX_VUS      VUs pré-alocados e teto
//   RUN_ID            rótulo da execução (vira label no Prometheus)
//   SUMMARY           caminho do resumo JSON escrito ao final

import http from 'k6/http';
import { sleep } from 'k6';
import exec from 'k6/execution';
import { Counter } from 'k6/metrics';

const TARGET = __ENV.TARGET || 'http://servico:8080/work';
const RATE = parseFloat(__ENV.RATE || '100');
const DURATION_S = parseInt(__ENV.DURATION_S || '30', 10);
const POLICY = __ENV.POLICY || 'fixed';
const MAX_RETRIES = POLICY === 'none' ? 0 : parseInt(__ENV.MAX_RETRIES || '3', 10);
const FIXED_DELAY_MS = parseFloat(__ENV.FIXED_DELAY_MS || '50');
const BACKOFF_BASE_MS = parseFloat(__ENV.BACKOFF_BASE_MS || '100');
const BACKOFF_CAP_MS = parseFloat(__ENV.BACKOFF_CAP_MS || '1000');
const ARRIVAL_JITTER_MS = parseFloat(__ENV.ARRIVAL_JITTER_MS || '1000');
const SEED = parseInt(__ENV.SEED || '27', 10);
const VUS = parseInt(__ENV.VUS || '200', 10);
const MAX_VUS = parseInt(__ENV.MAX_VUS || '400', 10);

if (!['none', 'immediate', 'fixed', 'backoff'].includes(POLICY)) {
  throw new Error(`POLICY inválida: ${POLICY}`);
}

// Classes: [{nome, timeout, acumulado}], com as frações acumuladas para o sorteio.
const CLASSES = (() => {
  const txt = __ENV.CLASSES || 'impatient:100:0.5,patient:200:0.5';
  let soma = 0;
  const lista = txt.split(',').map((p) => {
    const [nome, timeout, frac] = p.split(':');
    soma += parseFloat(frac);
    return { nome, timeout: parseInt(timeout, 10), acumulado: soma };
  });
  if (Math.abs(soma - 1) > 1e-6) throw new Error(`frações das classes somam ${soma}, não 1`);
  return lista;
})();

// Hash inteiro de 32 bits (finalizador do murmur3) → número em [0, 1).
function uniforme(a, b) {
  let h = (Math.imul(a, 0x9e3779b1) ^ Math.imul(b + 0x7f4a7c15, 0x85ebca6b)) | 0;
  h ^= h >>> 16;
  h = Math.imul(h, 0x85ebca6b);
  h ^= h >>> 13;
  h = Math.imul(h, 0xc2b2ae35);
  h ^= h >>> 16;
  return (h >>> 0) / 4294967296;
}

// Contadores do lado do cliente, com o rótulo "class". É aqui que o goodput é
// medido: só o cliente sabe se a resposta chegou dentro do prazo.
const originados = new Counter('bancada_originados'); // pedidos lógicos novos
const tentativas = new Counter('bancada_tentativas'); // tentativas HTTP enviadas
const sucesso = new Counter('bancada_sucesso'); // pedidos respondidos no prazo (goodput)
const timeouts = new Counter('bancada_timeouts'); // tentativas que estouraram o timeout
const rejeitados = new Counter('bancada_rejeitados'); // tentativas que receberam 503
const outros = new Counter('bancada_outros_erros'); // qualquer outra falha
const falhas = new Counter('bancada_falhas'); // pedidos que esgotaram as tentativas
const latenciaOkMs = new Counter('bancada_latencia_ok_ms'); // soma das latências das respostas no prazo

export const options = {
  scenarios: {
    aberto: {
      executor: 'constant-arrival-rate',
      rate: Math.round(RATE * 100),
      timeUnit: '100s',
      duration: `${DURATION_S}s`,
      preAllocatedVUs: VUS,
      maxVUs: MAX_VUS,
      gracefulStop: '5s',
    },
  },
  tags: { run: __ENV.RUN_ID || 'avulso' },
  discardResponseBodies: true,
  systemTags: ['scenario', 'status'],
};

export default function () {
  const iter = exec.scenario.iterationInTest;
  // Classe e deslocamento de chegada: números comuns entre configurações.
  const u = uniforme(SEED, 2 * iter);
  const classe = CLASSES.find((c) => u < c.acumulado) || CLASSES[CLASSES.length - 1];
  if (ARRIVAL_JITTER_MS > 0) {
    sleep((uniforme(SEED, 2 * iter + 1) * ARRIVAL_JITTER_MS) / 1000);
  }

  const tag = { class: classe.nome };
  const params = {
    timeout: `${classe.timeout}ms`,
    headers: { 'X-Budget-Ms': String(classe.timeout), 'X-Client-Class': classe.nome },
  };
  originados.add(1, tag);
  for (let tentativa = 0; ; tentativa++) {
    const res = http.get(TARGET, params);
    tentativas.add(1, tag);

    if (res.status === 200) {
      sucesso.add(1, tag);
      latenciaOkMs.add(res.timings.duration, tag);
      return;
    }
    if (res.status === 0) {
      timeouts.add(1, tag);
    } else if (res.status === 503) {
      rejeitados.add(1, tag);
    } else {
      outros.add(1, tag);
    }

    if (tentativa >= MAX_RETRIES) {
      falhas.add(1, tag);
      return;
    }
    if (POLICY === 'fixed') {
      sleep(FIXED_DELAY_MS / 1000);
    } else if (POLICY === 'backoff') {
      // Backoff exponencial com "full jitter": uniforme em [0, min(teto, base · 2^tentativa)).
      const teto = Math.min(BACKOFF_CAP_MS, BACKOFF_BASE_MS * Math.pow(2, tentativa));
      sleep((Math.random() * teto) / 1000);
    }
    // POLICY === 'immediate': tenta de novo na hora.
  }
}

export function handleSummary(data) {
  const saida = {};
  if (__ENV.SUMMARY) {
    saida[__ENV.SUMMARY] = JSON.stringify(data, null, 2);
  }
  return saida;
}
