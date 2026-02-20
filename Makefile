.PHONY: build run clean

build:
	go build -o portal .

run: build
	./portal

clean:
	rm -f portal
