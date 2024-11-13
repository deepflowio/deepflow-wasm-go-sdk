module github.com/threestoneliu/deepflow-llm

go 1.23.0

require (
	github.com/deepflowio/deepflow-wasm-go-sdk v0.0.0-00010101000000-000000000000
	github.com/valyala/fastjson v1.6.4
	github.com/wasilibs/nottinygc v0.7.1
)

require github.com/magefile/mage v1.14.0 // indirect

replace github.com/deepflowio/deepflow-wasm-go-sdk => ./deepflow-wasm-go-sdk-6.4
