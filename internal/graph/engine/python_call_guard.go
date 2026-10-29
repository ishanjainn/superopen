package engine

import "strings"

// These names are methods of Python's builtin containers, strings, and file
// objects. An untyped receiver does not prove a project method with the same
// name, including when the receiver is a field of self.
var pythonGenericBuiltinMembers = func() map[string]bool {
	out := map[string]bool{}
	for _, name := range strings.Fields(`add append capitalize casefold center clear close copy count decode
		difference discard encode endswith expandtabs extend fileno find flush format get index insert
		intersection isalnum isalpha isdecimal isdigit isidentifier islower isnumeric isprintable
		isspace istitle isupper issubset issuperset items join keys ljust lower lstrip partition
		pop popitem print read readable readline readlines remove replace reverse rfind rindex
		rjust rpartition rsplit rstrip seek setdefault sort split splitlines startswith strip
		swapcase symmetric_difference tell title truncate union update upper values writable
		write writelines zfill`) {
		out[name] = true
	}
	return out
}()

// pythonSuppressWeakMemberCall mirrors the call pass's receiver guard. A name
// guess is insufficient for an arbitrary x.method() receiver; an import-bound
// root or a unique non-builtin method under self.field supplies more evidence.
func pythonSuppressWeakMemberCall(callee, strategy string, imports map[string]string, external map[string]bool) bool {
	if !guessStrategy(strategy) {
		return false
	}
	lastDot := strings.LastIndexByte(callee, '.')
	if lastDot < 0 {
		return false
	}
	receiver, method := callee[:lastDot], callee[lastDot+1:]
	if receiver == "self" || receiver == "cls" || receiver == "super()" {
		return false
	}
	root := callee[:strings.IndexByte(callee, '.')]
	if imports[root] != "" || external[root] {
		return false
	}
	if strategy == "unique_name" && (root == "self" || root == "cls") &&
		!strings.ContainsAny(receiver, "()[]") && !pythonGenericBuiltinMembers[method] {
		return false
	}
	return true
}

// A parameter shadows a same-named function throughout its enclosing scope.
// A resolved callable alias carries stronger local evidence.
func pythonParameterShadowsCall(scope, callee, strategy string, parameters map[string]map[string]bool) bool {
	if strategy == "lsp_callable_alias" || strings.Contains(callee, ".") {
		return false
	}
	for scope != "" {
		if parameters[scope][callee] {
			return true
		}
		index := strings.LastIndexByte(scope, '.')
		if index < 0 {
			break
		}
		scope = scope[:index]
	}
	return false
}
