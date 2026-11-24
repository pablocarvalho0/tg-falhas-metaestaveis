// Serviço da bancada de falhas metaestáveis (Lab Exame CSC-27).
//
// Um servidor HTTP com um pool fixo de C workers e uma fila limitada de Q
// posições. Cada pedido aceito custa uma consulta ao Postgres que demora
// T ms (pg_sleep), então a capacidade é conhecida: μ ≈ C/T.
//
// Duas escolhas de projeto são deliberadas, porque sem elas o laço de retry
// não pode fechar (slide "Duas condições para o laço poder fechar"):
//
//  1. a fila é grande o bastante para atrasar um pedido além do timeout do
//     cliente (Q/μ + T > timeout);
//  2. não há cancelamento: se o cliente desiste, o pedido continua na fila e
//     é processado mesmo assim. Esse é o trabalho desperdiçado.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		log.Fatalf("variável %s inválida: %q", name, v)
	}
	return n
}

func envStr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// trabalhoPostgres devolve a função de trabalho real: uma consulta que lê uma
// linha e segura a conexão por serviceMs milissegundos dentro do banco.
func trabalhoPostgres(pool *pgxpool.Pool, serviceMs int) WorkFunc {
	const q = `SELECT (SELECT valor FROM itens WHERE id = $2) FROM pg_sleep($1)`
	segundos := float64(serviceMs) / 1000.0
	return func(ctx context.Context, seq uint64) error {
		var valor *string
		return pool.QueryRow(ctx, q, segundos, int(seq%10000)+1).Scan(&valor)
	}
}

// conectar espera o banco (via proxy) ficar disponível e abre exatamente um
// socket por worker, para que nenhum worker pague handshake durante a medição.
func conectar(ctx context.Context, url string, workers int) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = int32(workers)
	cfg.MinConns = int32(workers)
	// As conexões nunca são recicladas durante a medição. Atenção: no pgxpool,
	// 0 NÃO significa "sem limite" — significa "já expirada", e o pool passaria
	// a abrir uma conexão nova a cada pedido.
	cfg.MaxConnLifetime = 30 * 24 * time.Hour
	cfg.MaxConnIdleTime = 30 * 24 * time.Hour
	cfg.HealthCheckPeriod = time.Hour

	var pool *pgxpool.Pool
	for tentativa := 1; ; tentativa++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			err = pool.Ping(ctx)
		}
		if err == nil {
			break
		}
		if pool != nil {
			pool.Close()
		}
		if tentativa >= 60 {
			return nil, err
		}
		log.Printf("banco indisponível (tentativa %d): %v", tentativa, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}

	// Aquece todas as conexões: abre as C de uma vez e roda a consulta em cada
	// uma, para o plano ficar preparado antes do primeiro pedido medido.
	conns := make([]*pgxpool.Conn, 0, workers)
	for i := 0; i < workers; i++ {
		c, err := pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		conns = append(conns, c)
	}
	for _, c := range conns {
		var valor *string
		err := c.QueryRow(ctx, `SELECT (SELECT valor FROM itens WHERE id = $2) FROM pg_sleep($1)`, 0.0, 1).Scan(&valor)
		c.Release()
		if err != nil {
			return nil, err
		}
	}
	return pool, nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	workers := envInt("WORKERS", 8)
	fila := envInt("QUEUE", 64)
	serviceMs := envInt("SERVICE_MS", 40)
	addr := envStr("LISTEN", ":8080")
	dbURL := envStr("DB_URL", "postgres://bancada:bancada@toxiproxy:5433/bancada")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := conectar(ctx, dbURL, workers)
	if err != nil {
		log.Fatalf("não foi possível conectar ao banco: %v", err)
	}
	defer pool.Close()

	ms := func(nome string, def int) time.Duration { return time.Duration(envInt(nome, def)) * time.Millisecond }
	pol := Politica{
		Nome:      envStr("QUEUE_POLICY", PolFixa),
		MaxEspera: ms("MAX_WAIT_MS", 100),
		SHat:      ms("S_HAT_MS", 42),
		Margem:    ms("MARGIN_MS", 10),
		Janela:    ms("OVERLOAD_MS", 100),
		Detector:  envStr("OVERLOAD_DETECTOR", DetVazia),
		AlvoCoDel: ms("CODEL_TARGET_MS", 5),
	}
	s := NewServer(Config{Workers: workers, Queue: fila, ServiceMs: serviceMs, Politica: pol}, trabalhoPostgres(pool, serviceMs))
	if err := s.DefinirPolitica(pol); err != nil {
		log.Fatal(err)
	}
	s.Start()

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Printf("serviço no ar em %s: C=%d workers, Q=%d, T=%d ms (μ nominal ≈ %.0f req/s), política %s",
		addr, workers, fila, serviceMs, float64(workers)*1000/float64(serviceMs), pol.Nome)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
