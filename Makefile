.PHONY: build run clean wasm server

build: wasm server

wasm:
	GOARCH=wasm GOOS=js go build -o web/app.wasm ./app/

server:
	go build -o portal .

run: build
	./portal

clean:
	rm -f portal web/app.wasm
