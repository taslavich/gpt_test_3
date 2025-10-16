package coder

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

func AdmToAdidCompact(adm string) string {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	w.Write([]byte(adm))
	w.Close()

	encoded := base64.RawURLEncoding.EncodeToString(buf.Bytes())
	return "ad_" + encoded
}

func AdidToAdmCompact(adid string) (string, error) {
	if !strings.HasPrefix(adid, "ad_") {
		return "", errors.New("invalid adid format")
	}

	data, err := base64.RawURLEncoding.DecodeString(adid[3:])
	if err != nil {
		return "", err
	}

	buf := bytes.NewBuffer(data)
	r := flate.NewReader(buf)
	defer r.Close()

	result, err := io.ReadAll(r)
	return string(result), err
}
