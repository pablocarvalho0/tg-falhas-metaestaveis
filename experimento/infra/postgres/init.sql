-- Dados da bancada: cada pedido lê uma linha desta tabela.
CREATE TABLE itens (
    id    integer PRIMARY KEY,
    valor text NOT NULL
);
INSERT INTO itens (id, valor)
SELECT g, md5(g::text) FROM generate_series(1, 10000) AS g;
ANALYZE itens;
