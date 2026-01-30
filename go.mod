module title-parser

go 1.22

require (
	gopkg.in/yaml.v3 v3.0.1
	reltitle/training/titleparser v0.0.0
)

require (
	github.com/yalue/onnxruntime_go v1.14.0 // indirect
	golang.org/x/text v0.21.0 // indirect
)

replace reltitle/training/titleparser => ../training/go/titleparser
