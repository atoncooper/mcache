module github.com/atoncooper/mcache/tests/go

go 1.24.3

require github.com/atoncooper/mcache/sdk/go v0.0.0

require (
	github.com/atoncooper/mcache v0.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/atoncooper/mcache => ../../
	github.com/atoncooper/mcache/sdk/go => ../../sdk/go
)
