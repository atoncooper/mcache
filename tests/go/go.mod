module github.com/atoncooper/mcache/tests/go

go 1.24.3

require (
	github.com/atoncooper/mcache v0.1.0
	github.com/atoncooper/mcache/sdk/go v0.0.0
)

require gopkg.in/yaml.v3 v3.0.1 // indirect

replace (
	github.com/atoncooper/mcache => ../../
	github.com/atoncooper/mcache/sdk/go => ../../sdk/go
)
