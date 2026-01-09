package wrapper

import "github.com/sweemingdow/gmicro_pkg/pkg/parser/json"

const (
	Ok         = "1"
	GeneralErr = "0"
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

func (hrw HttpRespWrapper[T]) IsOK() bool {
	return hrw.Code == Ok
}

func (hrw HttpRespWrapper[T]) IsGeneralErr() bool {
	return hrw.Code == GeneralErr
}

func ParseResp[T any](respBuf []byte, vp *HttpRespWrapper[T]) error {
	if err := json.Parse(respBuf, vp); err != nil {
		return err
	}

	return nil
}
