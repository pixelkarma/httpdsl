.PHONY: all clean build-runtime build

# Build the standalone runtime first, then embed it in the compiler
all: build

# Step 1: Build the standalone runtime as a static, stripped binary
build-runtime:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o runtime.bin ./cmd/standalone/
	@echo "Runtime binary: $$(ls -lh runtime.bin | awk '{print $$5}')"

# Step 2: Build httpdsl with the embedded runtime
build: build-runtime
	go build -ldflags="-s -w" -o httpdsl .
	@echo "Compiler binary: $$(ls -lh httpdsl | awk '{print $$5}')"
	@echo "Done. Run: ./httpdsl build <file.httpdsl>"

clean:
	rm -f runtime.bin httpdsl
