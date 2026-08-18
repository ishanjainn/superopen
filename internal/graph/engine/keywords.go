package engine

import "strings"

var genericKeywords = keywordSet(
	"true false null nil None undefined void if else for while do switch case default break continue return throw try catch finally class struct enum interface trait impl import export package module use require include new delete this self super public private protected static const var let function def fn func fun proc sub method async await yield",
)

var languageKeywords = map[string]map[string]bool{
	"go":         keywordSet("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var true false nil iota append cap close complex copy delete imag len make new panic print println real recover"),
	"python":     keywordSet("False None True and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield self cls __init__ __name__ __main__ super print len range enumerate zip map filter type int str float bool list dict set tuple bytes"),
	"javascript": keywordSet("break case catch class const continue debugger default delete do else export extends false finally for function if import in instanceof let new null return super switch this throw true try typeof undefined var void while with yield async await of static get set from as constructor prototype console window document process module exports require Array Object String Number Boolean Symbol Map Set Promise Error RegExp Date Math JSON parseInt parseFloat setTimeout setInterval clearTimeout clearInterval"),
	"rust":       keywordSet("as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while abstract become box do final macro override priv try typeof unsized virtual yield Some None Ok Err Vec String Box Rc Arc Option Result println eprintln format write writeln print eprint panic assert assert_eq assert_ne debug_assert todo unimplemented cfg derive test allow deny warn forbid deprecated"),
	"java":       keywordSet("abstract assert boolean break byte case catch char class const continue default do double else enum extends false final finally float for goto if implements import instanceof int interface long native new null package private protected public return short static strictfp super switch synchronized this throw throws transient true try void volatile while var record sealed permits yield System String Integer Long Double Float Boolean Object List Map Set Optional Stream Arrays Collections"),
	"kotlin":     keywordSet("as break class continue do else false for fun if in interface is null object package return super this throw true try typealias typeof val var when while"),
	"puppet":     keywordSet("true false undef if elsif else unless case and or in node class define inherits default return"),
}

var pythonResolvableBuiltins = keywordSet("len print str int list dict range")

func keywordSet(value string) map[string]bool {
	result := map[string]bool{}
	for _, word := range strings.Fields(value) {
		result[word] = true
	}
	return result
}

func isLanguageKeyword(language, name string) bool {
	if name == "" {
		return true
	}
	set := languageKeywords[language]
	if language == "typescript" || language == "tsx" {
		set = languageKeywords["javascript"]
	} else if language == "scala" {
		set = languageKeywords["java"]
	}
	if set == nil {
		set = genericKeywords
	}
	return set[name]
}

func isResolvableBuiltin(language, name string) bool {
	return language == "python" && pythonResolvableBuiltins[name]
}
