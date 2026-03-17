package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type RunOptions struct {
	ForceSilent bool
}

type RunResult struct {
	Body   []byte
	Status int
	Stderr []byte
}

func Run(args []string, opts RunOptions) (RunResult, error) {
	if len(args) == 0 {
		return RunResult{}, errors.New("missing curl arguments")
	}

	cmdArgs := append([]string{}, args...)
	if opts.ForceSilent && !hasSilent(cmdArgs) {
		cmdArgs = append(cmdArgs, "-sS")
	}
	cmdArgs = append(cmdArgs, "-w", "\n__JSC_STATUS__:%{http_code}")

	stdout, stderr, err := runCurl(cmdArgs)
	body, status, splitErr := splitStatus(stdout)
	if splitErr != nil {
		return RunResult{Body: stdout, Status: 0, Stderr: stderr}, splitErr
	}

	if err != nil {
		return RunResult{Body: body, Status: status, Stderr: stderr}, err
	}

	return RunResult{
		Body:   stripHTTPHeaders(body),
		Status: status,
		Stderr: stderr,
	}, nil
}

type Header struct {
	Key string
	Raw string
}

func ExtractHeaders(args []string) []Header {
	var headers []Header
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-H", "--header":
			if i+1 < len(args) {
				raw := args[i+1]
				if key := headerKey(raw); key != "" {
					headers = append(headers, Header{Key: key, Raw: raw})
				}
				i++
			}
		default:
			if strings.HasPrefix(args[i], "-H") && len(args[i]) > 2 {
				raw := args[i][2:]
				if key := headerKey(raw); key != "" {
					headers = append(headers, Header{Key: key, Raw: raw})
				}
			}
			if strings.HasPrefix(args[i], "--header=") {
				raw := strings.TrimPrefix(args[i], "--header=")
				if key := headerKey(raw); key != "" {
					headers = append(headers, Header{Key: key, Raw: raw})
				}
			}
		}
	}
	return headers
}

func RemoveHeaders(args []string, removeKeys map[string]bool) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-H", "--header":
			if i+1 < len(args) {
				raw := args[i+1]
				if key := headerKey(raw); key != "" && removeKeys[key] {
					i++
					continue
				}
				out = append(out, arg, raw)
				i++
				continue
			}
		default:
			if strings.HasPrefix(arg, "-H") && len(arg) > 2 {
				raw := arg[2:]
				if key := headerKey(raw); key != "" && removeKeys[key] {
					continue
				}
				out = append(out, arg)
				continue
			}
			if strings.HasPrefix(arg, "--header=") {
				raw := strings.TrimPrefix(arg, "--header=")
				if key := headerKey(raw); key != "" && removeKeys[key] {
					continue
				}
				out = append(out, arg)
				continue
			}
		}
		out = append(out, arg)
	}
	return out
}

// ReplaceHeaderValue 替换指定 header key 的值
func ReplaceHeaderValue(args []string, targetKey string, newValue string) []string {
	out := make([]string, 0, len(args))
	replaced := false
	
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-H", "--header":
			if i+1 < len(args) {
				raw := args[i+1]
				if key := headerKey(raw); key != "" && strings.EqualFold(key, targetKey) && !replaced {
					// 替换这个 header 的值
					out = append(out, arg, key+": "+newValue)
					i++
					replaced = true
					continue
				}
				out = append(out, arg, raw)
				i++
				continue
			}
		default:
			if strings.HasPrefix(arg, "-H") && len(arg) > 2 {
				raw := arg[2:]
				if key := headerKey(raw); key != "" && strings.EqualFold(key, targetKey) && !replaced {
					out = append(out, "-H", key+": "+newValue)
					replaced = true
					continue
				}
				out = append(out, arg)
				continue
			}
			if strings.HasPrefix(arg, "--header=") {
				raw := strings.TrimPrefix(arg, "--header=")
				if key := headerKey(raw); key != "" && strings.EqualFold(key, targetKey) && !replaced {
					out = append(out, "--header="+key+": "+newValue)
					replaced = true
					continue
				}
				out = append(out, arg)
				continue
			}
		}
		out = append(out, arg)
	}
	
	return out
}

type DataRef struct {
	Index  int
	Kind   string // next|inline|eq 表示参数形态
	Prefix string
	Value  string
}

func FindFirstDataArg(args []string) (DataRef, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-d", "--data", "--data-raw", "--data-binary", "--data-urlencode":
			if i+1 < len(args) {
				return DataRef{Index: i + 1, Kind: "next", Prefix: arg, Value: args[i+1]}, true
			}
		default:
			if strings.HasPrefix(arg, "-d") && len(arg) > 2 {
				return DataRef{Index: i, Kind: "inline", Prefix: "-d", Value: arg[2:]}, true
			}
			if strings.HasPrefix(arg, "--data=") {
				return DataRef{Index: i, Kind: "eq", Prefix: "--data=", Value: strings.TrimPrefix(arg, "--data=")}, true
			}
			if strings.HasPrefix(arg, "--data-raw=") {
				return DataRef{Index: i, Kind: "eq", Prefix: "--data-raw=", Value: strings.TrimPrefix(arg, "--data-raw=")}, true
			}
			if strings.HasPrefix(arg, "--data-binary=") {
				return DataRef{Index: i, Kind: "eq", Prefix: "--data-binary=", Value: strings.TrimPrefix(arg, "--data-binary=")}, true
			}
			if strings.HasPrefix(arg, "--data-urlencode=") {
				return DataRef{Index: i, Kind: "eq", Prefix: "--data-urlencode=", Value: strings.TrimPrefix(arg, "--data-urlencode=")}, true
			}
		}
	}
	return DataRef{}, false
}

func ReplaceDataArg(args []string, ref DataRef, value string) []string {
	out := append([]string{}, args...)
	switch ref.Kind {
	case "next":
		if ref.Index >= 0 && ref.Index < len(out) {
			out[ref.Index] = value
		}
	case "inline":
		if ref.Index >= 0 && ref.Index < len(out) {
			out[ref.Index] = ref.Prefix + value
		}
	case "eq":
		if ref.Index >= 0 && ref.Index < len(out) {
			out[ref.Index] = ref.Prefix + value
		}
	}
	return out
}

func FindFirstURL(args []string) (string, int) {
	for i := len(args) - 1; i >= 0; i-- {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if strings.Contains(arg, "://") || strings.HasPrefix(arg, "http") {
			return arg, i
		}
	}
	return "", -1
}

func hasSilent(args []string) bool {
	for _, a := range args {
		if a == "-s" || a == "--silent" || a == "-sS" {
			return true
		}
	}
	return false
}

func headerKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[0]))
}

func splitStatus(stdout []byte) ([]byte, int, error) {
	marker := []byte("\n__JSC_STATUS__:")
	idx := bytes.LastIndex(stdout, marker)
	if idx == -1 {
		return stdout, 0, errors.New("missing status marker")
	}
	body := stdout[:idx]
	statusRaw := strings.TrimSpace(string(stdout[idx+len(marker):]))
	if statusRaw == "" {
		return body, 0, errors.New("missing status code")
	}
	var status int
	if _, err := fmt.Sscanf(statusRaw, "%d", &status); err != nil {
		return body, 0, fmt.Errorf("invalid status code: %w", err)
	}
	return body, status, nil
}

func stripHTTPHeaders(body []byte) []byte {
	trimmed := bytes.TrimLeft(body, " \r\n\t")
	if !bytes.HasPrefix(trimmed, []byte("HTTP/")) {
		return body
	}
	if idx := bytes.LastIndex(body, []byte("\r\n\r\n")); idx != -1 {
		return body[idx+4:]
	}
	if idx := bytes.LastIndex(body, []byte("\n\n")); idx != -1 {
		return body[idx+2:]
	}
	return body
}

func runCurl(args []string) ([]byte, []byte, error) {
	cmd := exec.Command("curl", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}
