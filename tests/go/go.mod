module github.com/atoncooper/mcache/tests/go

go 1.24.3

require (
	github.com/atoncooper/mcache v0.1.0
	github.com/atoncooper/mcache/sdk/go v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/redis/go-redis/v9 v9.19.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/atoncooper/mcache => ../../
	github.com/atoncooper/mcache/sdk/go => ../../sdk/go
)
