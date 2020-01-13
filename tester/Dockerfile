FROM python:3.8.1-alpine

WORKDIR /app

RUN pip install pytest requests

COPY tests/ /app

CMD ["pytest"]