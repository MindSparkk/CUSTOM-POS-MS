FROM golang:alpine AS builder

WORKDIR /app

# Copy the whole workspace
COPY . .

# Build the specific service passed via ARG
ARG SERVICE_NAME
RUN cd services/${SERVICE_NAME} && go build -o /bin/service .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /bin/service /app/service

CMD ["/app/service"]
