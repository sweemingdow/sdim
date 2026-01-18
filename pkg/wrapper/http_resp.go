package wrapper

import "github.com/sweemingdow/gmicro_pkg/pkg/parser/json"

const (
	Ok     = "1"
	GenErr = "0"
)

type HttpRespWrapper[T any] struct {
	Code    string `json:"code,omitempty"`
	SubCode string `json:"subCode,omitempty"`
	Msg     string `json:"msg,omitempty"`
	Data    T      `json:"data,omitempty"`
}

func RespOk[T any](data T) HttpRespWrapper[T] {
	return HttpRespWrapper[T]{
		Code: Ok,
		Data: data,
	}
}

func JustOk() HttpRespWrapper[any] {
	return HttpRespWrapper[any]{
		Code: Ok,
	}
}

func JustGeneralErr() HttpRespWrapper[any] {
	return HttpRespWrapper[any]{
		Code: GenErr,
	}
}

func GeneralErr(err error) HttpRespWrapper[any] {
	return HttpRespWrapper[any]{
		Code: GenErr,
		Msg:  err.Error(),
	}
}

func RespGeneralErrAll[T any](code, subCode, msg string, data T) HttpRespWrapper[T] {
	return HttpRespWrapper[T]{
		Code:    GenErr,
		SubCode: subCode,
		Msg:     msg,
		Data:    data,
	}
}

func (hrw HttpRespWrapper[T]) IsOK() bool {
	return hrw.Code == Ok
}

func (hrw HttpRespWrapper[T]) IsGeneralErr() bool {
	return hrw.Code == GenErr
}

func ParseResp[T any](respBuf []byte, vp *HttpRespWrapper[T]) error {
	if err := json.Parse(respBuf, vp); err != nil {
		return err
	}

	return nil
}
