FROM python:3.13-slim
RUN pip install --no-cache-dir matplotlib==3.10.7 numpy==2.3.4
ENV MPLBACKEND=Agg MPLCONFIGDIR=/tmp/mpl PYTHONDONTWRITEBYTECODE=1
COPY analise.py /analise/analise.py
WORKDIR /resultados
ENTRYPOINT ["python", "/analise/analise.py"]
