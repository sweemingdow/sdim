package fcodec

import "github.com/sweemingdow/gmicro_pkg/pkg/parser/json"

func EncodePayload(pl Payload, body any) (Payload, error) {
	if pl.PayloadProtocol == JsonPayload {
		bodies, err := json.Fmt(body)
		if err != nil {
			return emptyPayload, err
		}

		return Payload{
			PayloadProtocol: pl.PayloadProtocol,
			ReqId:           pl.ReqId,
			Body:            bodies,
		}, nil

	}

	panic("unsupported PayloadProtocType")
}

func DecodePayload(pl Payload, val any) error {
	if pl.PayloadProtocol == JsonPayload {
		err := json.Parse(pl.Body, val)
		if err != nil {
			return err
		}

		return nil
	}

	panic("unsupported PayloadProtocType")
}
