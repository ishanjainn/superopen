package engine

import "strings"

const semanticTokenLimit = 512

// semanticAbbreviations is pinned to the Superopen semantic tokenizer. Expansion
// is additive and only examines original tokens, preserving order and limits.
var semanticAbbreviations = map[string]string{
	"err": "error", "exc": "exception", "ex": "exception",
	"ctx": "context", "cfg": "config", "conf": "configuration", "env": "environment", "opt": "option", "opts": "options",
	"req": "request", "res": "response", "resp": "response", "rsp": "response", "hdr": "header", "hdrs": "headers",
	"str": "string", "fmt": "format", "msg": "message", "txt": "text", "lbl": "label", "desc": "description",
	"buf": "buffer", "arr": "array", "vec": "vector", "lst": "list", "dict": "dictionary", "tbl": "table", "stk": "stack", "que": "queue",
	"fn": "function", "func": "function", "cb": "callback", "proc": "procedure", "ctor": "constructor", "dtor": "destructor",
	"db": "database", "col": "column", "stmt": "statement", "txn": "transaction", "trx": "transaction", "repo": "repository",
	"auth": "authentication", "authz": "authorization", "perm": "permission", "cred": "credential", "tok": "token", "pwd": "password",
	"val": "value", "num": "number", "int": "integer", "bool": "boolean", "flt": "float", "dbl": "double",
	"idx": "index", "iter": "iterator", "elem": "element", "cnt": "count", "len": "length", "sz": "size", "pos": "position", "off": "offset", "cap": "capacity",
	"init": "initialize", "deinit": "deinitialize", "alloc": "allocate", "dealloc": "deallocate", "del": "delete", "rm": "remove",
	"impl": "implementation", "iface": "interface", "abs": "abstract", "decl": "declaration",
	"param": "parameter", "arg": "argument", "attr": "attribute", "prop": "property", "ret": "return",
	"src": "source", "dst": "destination", "tgt": "target", "orig": "original", "prev": "previous", "cur": "current", "tmp": "temporary", "temp": "temporary",
	"conn": "connection", "sess": "session", "sock": "socket", "addr": "address", "url": "uniform", "srv": "server", "cli": "client", "svc": "service", "ep": "endpoint",
	"mgr": "manager", "ctrl": "controller", "hdlr": "handler", "sched": "scheduler", "disp": "dispatcher", "reg": "registry",
	"chan": "channel", "sem": "semaphore", "mtx": "mutex", "wg": "waitgroup", "sig": "signal", "evt": "event", "sub": "subscriber", "pub": "publisher",
	"spec": "specification", "mock": "mock", "stub": "stub", "assert": "assertion",
	"log": "logging", "lvl": "level", "dbg": "debug", "wrn": "warning", "inf": "info",
	"ts": "timestamp", "dur": "duration", "ttl": "timetolive",
	"ver": "version", "ns": "namespace", "pkg": "package", "mod": "module", "lib": "library", "dep": "dependency",
	"ref": "reference", "ptr": "pointer", "obj": "object", "doc": "document", "cmd": "command", "ops": "operations",
	"util": "utility", "hlp": "helper", "ext": "extension",
}

// semanticTokens matches Superopen's ASCII tokenization: delimiter and
// lower-to-upper camel boundaries, lowercase alphanumerics, 127-byte token
// truncation, then abbreviation expansion.
func semanticTokens(name string, maxTokens int) []string {
	if maxTokens <= 0 {
		return nil
	}
	if maxTokens > semanticTokenLimit {
		maxTokens = semanticTokenLimit
	}
	result := make([]string, 0, minInt(maxTokens, 16))
	var token strings.Builder
	flush := func() {
		if token.Len() > 0 && len(result) < maxTokens {
			result = append(result, token.String())
		}
		token.Reset()
	}
	for index := 0; index < len(name) && len(result) < maxTokens; index++ {
		char := name[index]
		delimiter := char == '.' || char == '/' || char == '_' || char == '-' || char == ' ' ||
			char == '(' || char == ')' || char == ',' || char == ':'
		camel := index > 0 && char >= 'A' && char <= 'Z' && name[index-1] >= 'a' && name[index-1] <= 'z'
		if delimiter || camel {
			flush()
			if delimiter {
				continue
			}
		}
		if token.Len() < 127 && (char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			if char >= 'A' && char <= 'Z' {
				char += 'a' - 'A'
			}
			token.WriteByte(char)
		}
	}
	flush()
	original := len(result)
	for index := 0; index < original && len(result) < maxTokens; index++ {
		if expanded := semanticAbbreviations[result[index]]; expanded != "" {
			result = append(result, expanded)
		}
	}
	return result
}
