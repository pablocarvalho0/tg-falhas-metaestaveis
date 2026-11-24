FROM ghcr.io/shopify/toxiproxy:2.12.0
COPY toxiproxy.json /config/toxiproxy.json
EXPOSE 8474 5433
CMD ["-host=0.0.0.0", "-config=/config/toxiproxy.json"]
