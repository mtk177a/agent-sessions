package chatgpt

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
)

type messageRole uint8

const (
	roleUnknown messageRole = iota
	roleMessage
	roleTool
)

func classifyRole(role string) messageRole {
	switch role {
	case "user", "assistant":
		return roleMessage
	case "tool":
		return roleTool
	default:
		return roleUnknown
	}
}

func isTextContent(kind string) bool {
	return kind == "text" || kind == "multimodal_text"
}

func (a *Adapter) lastInteractionAt(conversation rawConversation) (string, bool, error) {
	nodes, _, supported, err := a.activeNodes(conversation)
	if err != nil || !supported {
		return "", false, err
	}
	var latest time.Time
	found := false
	for _, node := range nodes {
		if len(node.Message) == 0 {
			return "", false, nil
		}
		if string(node.Message) == "null" {
			continue
		}
		var message rawMessage
		if json.Unmarshal(node.Message, &message) != nil {
			return "", false, nil
		}
		role := classifyRole(message.Author.Role)
		if role == roleUnknown {
			return "", false, nil
		}
		if role == roleMessage && message.Author.Role == "assistant" && (message.Content.ContentType == "thoughts" || message.Content.ContentType == "reasoning_recap") {
			continue
		}
		if !isTextContent(message.Content.ContentType) {
			return "", false, nil
		}
		at, ok := parseUnixSeconds(message.CreateTime)
		if !ok {
			return "", false, nil
		}
		if !found || at.After(latest) {
			latest = at
		}
		found = true
	}
	if !found {
		return "", false, nil
	}
	return latest.UTC().Format(time.RFC3339Nano), true, nil
}

func parseUnixSeconds(raw json.RawMessage) (time.Time, bool) {
	if len(raw) == 0 || len(raw) > 64 || raw[0] != '-' && (raw[0] < '0' || raw[0] > '9') {
		return time.Time{}, false
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil || number == "" {
		return time.Time{}, false
	}
	if exponent := strings.IndexAny(number.String(), "eE"); exponent >= 0 {
		value, err := strconv.ParseInt(number.String()[exponent+1:], 10, 64)
		if err != nil || value < -128 || value > 128 {
			return time.Time{}, false
		}
	}
	approx, err := strconv.ParseFloat(number.String(), 64)
	if err != nil || math.IsInf(approx, 0) || math.IsNaN(approx) || approx < -1e12 || approx > 1e12 {
		return time.Time{}, false
	}
	seconds, ok := new(big.Rat).SetString(number.String())
	if !ok {
		return time.Time{}, false
	}
	nanos := new(big.Rat).Mul(seconds, big.NewRat(1_000_000_000, 1))
	if !nanos.IsInt() {
		return time.Time{}, false
	}
	whole := new(big.Int)
	fraction := new(big.Int)
	whole.QuoRem(nanos.Num(), big.NewInt(1_000_000_000), fraction)
	if !whole.IsInt64() {
		return time.Time{}, false
	}
	at := time.Unix(whole.Int64(), fraction.Int64()).UTC()
	if at.Year() < 1 || at.Year() > 9999 {
		return time.Time{}, false
	}
	return at, true
}
