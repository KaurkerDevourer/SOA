FROM golang:1.18

WORKDIR /app

# Copy application source.
COPY . .

RUN mkdir -p bin

# Download dependencies.
RUN go mod download && go mod verify

# Build executable
RUN go build -o bin/client client/main.go

CMD ["./bin/client"]