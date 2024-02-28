# hostd pin
Pins storage, ingress, and egress price to fiat values

## Build
```sh
go build -o bin/ ./cmd/hpind
```

## Configure
```yaml
currency: usd
frequency: 5m
threshold: 0.05
prices:
	storage: 1.00
	ingress: 1.00
	egress: 1.00
hosts:
  - address: http://localhost:9980/api
    password: sia is cool
```

## Run
```sh
hpind --config config.yaml
```
