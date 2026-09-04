package safeio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func DecodeJSON(data []byte, maxDepth int, target any) error {
	if err := validateDepth(data, maxDepth); err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func DecodeJSONFile(path string, maxBytes int64, maxDepth int, target any) error {
	data, err := readRegularFile(path, maxBytes)
	if err != nil {
		return err
	}
	if err := validateDepth(data, maxDepth); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("input contains multiple JSON values")
	}
	return nil
}

func validateDepth(data []byte, maxDepth int) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			continue
		}
		switch delimiter {
		case '{', '[':
			depth++
			if depth > maxDepth {
				return fmt.Errorf("JSON nesting exceeds depth limit %d", maxDepth)
			}
		case '}', ']':
			depth--
		}
	}
}
