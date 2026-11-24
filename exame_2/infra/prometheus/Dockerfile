FROM prom/prometheus:v3.5.0
COPY prometheus.yml /etc/prometheus/prometheus.yml
# O k6 envia as métricas do cliente por remote write, a cada 1 s.
CMD ["--config.file=/etc/prometheus/prometheus.yml", \
     "--storage.tsdb.path=/prometheus", \
     "--storage.tsdb.retention.time=2d", \
     "--web.enable-remote-write-receiver"]
