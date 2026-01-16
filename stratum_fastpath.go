package main

import "bytes"

type jsonTopLevelKey int

const (
	jsonKeyOther jsonTopLevelKey = iota
	jsonKeyID
	jsonKeyMethod
)

// fastMiningSubmitID attempts to extract the JSON-RPC "id" token from a
// mining.submit request without fully unmarshaling the message. It returns
// ok=false if the message is not a mining.submit or if no id was found.
//
// idRaw is a slice of the original input containing the raw JSON token for id
// (e.g. 1, "abc", null). It is safe to embed directly into a JSON response.
func fastMiningSubmitID(b []byte) (idRaw []byte, ok bool) {
	// Quick filter to avoid scanning most non-submit messages.
	if !bytes.Contains(b, []byte("mining.submit")) {
		return nil, false
	}

	depth := 0
	inStr := false
	esc := false

	var (
		foundMethod bool
		methodOK    bool
	)

	for i := 0; i < len(b); i++ {
		c := b[i]

		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}

		switch c {
		case '"':
			// Only consider top-level object keys.
			if depth == 1 {
				k, next, okKey := scanTopLevelKey(b, i)
				if !okKey {
					return nil, false
				}
				i = next
				i = skipSpaces(b, i)
				if i >= len(b) || b[i] != ':' {
					return nil, false
				}
				i++
				i = skipSpaces(b, i)
				if i >= len(b) {
					return nil, false
				}

				switch k {
				case jsonKeyMethod:
					val, j, okVal := scanJSONValueToken(b, i)
					if !okVal || len(val) < 2 || val[0] != '"' || val[len(val)-1] != '"' {
						return nil, false
					}
					// "method" is ASCII for our use; compare without unescaping.
					methodOK = bytes.Equal(val[1:len(val)-1], []byte("mining.submit"))
					foundMethod = true
					i = j - 1
				case jsonKeyID:
					val, j, okVal := scanJSONValueToken(b, i)
					if !okVal {
						return nil, false
					}
					idRaw = val
					i = j - 1
				default:
					_, j, okVal := scanJSONValueToken(b, i)
					if !okVal {
						return nil, false
					}
					i = j - 1
				}

				if idRaw != nil && foundMethod {
					return idRaw, methodOK
				}
				continue
			}
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			}
		}
	}

	return nil, false
}

func scanTopLevelKey(b []byte, i int) (k jsonTopLevelKey, next int, ok bool) {
	// b[i] must be '"'
	start := i + 1
	j := start
	for ; j < len(b); j++ {
		if b[j] == '"' {
			break
		}
		if b[j] == '\\' {
			// Keys we care about don't contain escapes; treat as non-match but valid.
			return jsonKeyOther, j + 1, true
		}
	}
	if j >= len(b) || b[j] != '"' {
		return jsonKeyOther, j, false
	}
	keyBytes := b[start:j]
	switch {
	case bytes.Equal(keyBytes, []byte("id")):
		k = jsonKeyID
	case bytes.Equal(keyBytes, []byte("method")):
		k = jsonKeyMethod
	default:
		k = jsonKeyOther
	}
	return k, j + 1, true
}

func skipSpaces(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\r', '\n':
			i++
		default:
			return i
		}
	}
	return i
}

// scanJSONValueToken returns the raw slice for the next JSON value token
// starting at i, and the index immediately after the token.
func scanJSONValueToken(b []byte, i int) (tok []byte, next int, ok bool) {
	if i >= len(b) {
		return nil, i, false
	}
	switch b[i] {
	case '"':
		j, ok := scanJSONStringEnd(b, i)
		if !ok {
			return nil, i, false
		}
		return b[i:j], j, true
	case '{', '[':
		j, ok := scanJSONCompoundEnd(b, i)
		if !ok {
			return nil, i, false
		}
		return b[i:j], j, true
	default:
		// number / null / true / false
		j := i
		for j < len(b) {
			c := b[j]
			if c == ',' || c == '}' || c == ']' || c == ' ' || c == '\t' || c == '\r' || c == '\n' {
				break
			}
			j++
		}
		if j == i {
			return nil, i, false
		}
		return b[i:j], j, true
	}
}

func scanJSONStringEnd(b []byte, i int) (end int, ok bool) {
	// b[i] == '"'
	esc := false
	for j := i + 1; j < len(b); j++ {
		c := b[j]
		if esc {
			esc = false
			continue
		}
		if c == '\\' {
			esc = true
			continue
		}
		if c == '"' {
			return j + 1, true
		}
	}
	return i, false
}

func scanJSONCompoundEnd(b []byte, i int) (end int, ok bool) {
	// b[i] is '{' or '['
	depth := 0
	inStr := false
	esc := false
	for j := i; j < len(b); j++ {
		c := b[j]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return j + 1, true
			}
		}
	}
	return i, false
}
