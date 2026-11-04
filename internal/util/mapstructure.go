package util

import (
	"time"

	"github.com/mitchellh/mapstructure"
)

func MapToStruct[T any](input map[string]any, hooks ...mapstructure.DecodeHookFunc) (T, error) {
	return Decode[T](input, hooks...)
}

func Decode[T any](input any, hooks ...mapstructure.DecodeHookFunc) (T, error) {
	var result T

	defaultHooks := []mapstructure.DecodeHookFunc{
		mapstructure.StringToTimeHookFunc(time.RFC3339),
	}

	config := &mapstructure.DecoderConfig{
		Metadata: nil,
		Result:   &result,
		// TagName 默认为 "mapstructure"，可以改为 "json" 以复用现有的 json 标签
		TagName: "json",
		// WeaklyTypedInput 允许弱类型转换（如 string "123" -> int 123）
		WeaklyTypedInput: true,
		DecodeHook:       mapstructure.ComposeDecodeHookFunc(append(defaultHooks, hooks...)...),
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return result, err
	}

	err = decoder.Decode(input)
	return result, err
}
