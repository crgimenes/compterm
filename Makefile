all:
	go build -ldflags '-s -w'

dev:
	go run -tags dev .

