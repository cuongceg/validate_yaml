package router

import (
	"encoding/json"
	"fmt"
	"reflect"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func newOf[T any]() T {
	rt := reflect.TypeOf((*T)(nil)).Elem()
	if rt.Kind() == reflect.Pointer {
		return reflect.New(rt.Elem()).Interface().(T)
	}
	return reflect.New(rt).Elem().Interface().(T)
}

type ProtoCodec[T proto.Message] struct{}

func NewProtoCodec[T proto.Message]() *ProtoCodec[T] { return &ProtoCodec[T]{} }

func (c *ProtoCodec[T]) ContentType() string { return "application/x-protobuf" }

func (c *ProtoCodec[T]) Decode(b []byte) (map[string]any, any, error) {
	var zeroMap map[string]any
	msg := newOf[T]()
	if err := proto.Unmarshal(b, msg); err != nil {
		return nil, nil, err
	}
	jb, err := protojson.Marshal(msg)
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(jb, &zeroMap); err != nil {
		return nil, nil, err
	}
	return zeroMap, msg, nil
}

func (c *ProtoCodec[T]) Encode(obj map[string]any, orig any) ([]byte, error) {
	msg, ok := orig.(T)
	if !ok {
		return nil, fmt.Errorf("ProtoCodec: unexpected domain type (got %T)", orig)
	}
	jb, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	if err := protojson.Unmarshal(jb, msg); err != nil {
		return nil, err
	}
	return proto.Marshal(msg)
}
