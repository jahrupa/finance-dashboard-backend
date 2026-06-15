.PHONY: run build tidy clean

# Run the server in development mode
run:
	go run cmd/main.go

# Download / tidy dependencies
tidy:
	go mod tidy

# Build production binary
build:
	go build -o ap-backend cmd/main.go

# Clean build artifacts
clean:
	rm -f ap-backend
