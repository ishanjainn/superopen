package engine

import (
	"strings"
	"testing"
)

func TestTranscodeObjectScriptExportPinnedSurface(t *testing.T) {
	t.Parallel()
	xml := `<?xml version="1.0"?><Export generator="Cache"><Class name="Test.Members"><Super>A,B</Super><Abstract>1</Abstract><Method name="Run"><ClassMethod>1</ClassMethod><FormalSpec>arg:%String</FormalSpec><ReturnType>%Status</ReturnType><Implementation><![CDATA[ Quit $$$OK
]]></Implementation></Method><Property name="Name"><Type>%String</Type><Parameter name="MAXLEN" value="200"/></Property><Parameter name="VERSION"><Default>1</Default></Parameter><Index name="NameIdx"><Properties>Name</Properties><Unique>1</Unique></Index><XData name="Map"><Data><![CDATA[<x/>]]></Data></XData></Class></Export>`
	got := TranscodeObjectScriptExport([]byte(xml))
	if len(got) != 1 {
		t.Fatalf("classes = %d", len(got))
	}
	for _, want := range []string{"Class Test.Members Extends (A,B) [ Abstract ]", "ClassMethod Run(arg:%String) As %Status", "Property Name As %String(MAXLEN = 200);", "Parameter VERSION = \"1\";", "Index NameIdx On Name [ Unique ];", "XData Map", "<x/>"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("UDL missing %q:\n%s", want, got[0])
		}
	}
}

func TestTranscodeObjectScriptExportMultipleAndMalformed(t *testing.T) {
	t.Parallel()
	got := TranscodeObjectScriptExport([]byte(`<Export generator="Cache"><Class name="One"></Class><Class name="Two"></Class></Export>`))
	if len(got) != 2 || !strings.Contains(got[0], "Class One") || !strings.Contains(got[1], "Class Two") {
		t.Fatalf("classes = %#v", got)
	}
	if got := TranscodeObjectScriptExport([]byte(`<Class name="NoExport"></Class>`)); got != nil {
		t.Fatalf("non-export = %#v", got)
	}
}
