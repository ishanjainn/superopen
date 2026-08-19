package engine

import "strings"

const (
	objectScriptExportClasses = 64
	objectScriptUDLCap        = 64 << 10
)

// TranscodeObjectScriptExport reproduces the pinned Export XML to UDL bridge.
// It deliberately uses the same bounded, forgiving string scan as Superopen:
// malformed members are skipped and at most 64 classes are returned.
func TranscodeObjectScriptExport(source []byte) []string {
	xml := string(source)
	if !strings.Contains(xml, "<Export generator=") {
		return nil
	}
	result := []string{}
	for rest := xml; len(result) < objectScriptExportClasses; {
		start := strings.Index(rest, "<Class ")
		if start < 0 {
			break
		}
		rest = rest[start:]
		openEnd := strings.IndexByte(rest, '>')
		if openEnd < 0 {
			break
		}
		closeAt := strings.Index(rest[openEnd+1:], "</Class>")
		if closeAt < 0 {
			break
		}
		closeAt += openEnd + 1
		class := rest[:closeAt]
		if udl := transcodeObjectScriptClass(class); udl != "" {
			result = append(result, udl)
		}
		rest = rest[closeAt+len("</Class>"):]
	}
	return result
}

type boundedUDL struct {
	value strings.Builder
}

func (b *boundedUDL) add(value string) {
	if value == "" || b.value.Len()+len(value)+1 >= objectScriptUDLCap {
		return
	}
	b.value.WriteString(value)
}

func transcodeObjectScriptClass(class string) string {
	openEnd := strings.IndexByte(class, '>')
	if openEnd < 0 {
		return ""
	}
	name := xmlAttribute(class[:openEnd], "name", 255)
	if name == "" {
		return ""
	}
	var output boundedUDL
	output.add("Class ")
	output.add(name)
	if super, ok := xmlContent(class, "Super", 1023); ok && super != "" {
		if strings.Contains(super, ",") {
			output.add(" Extends (")
			output.add(super)
			output.add(")")
		} else {
			output.add(" Extends ")
			output.add(super)
		}
	}
	pragmas := []string{}
	if xmlTagIsOne(class, "Abstract") {
		pragmas = append(pragmas, "Abstract")
	}
	if xmlTagIsOne(class, "Final") {
		pragmas = append(pragmas, "Final")
	}
	if len(pragmas) > 0 {
		output.add(" [ ")
		output.add(strings.Join(pragmas, ","))
		output.add(" ]")
	}
	output.add("\n{\n\n")
	for offset := openEnd + 1; offset < len(class); {
		next := strings.IndexByte(class[offset:], '<')
		if next < 0 {
			break
		}
		offset += next
		tag, kind, end, ok := nextObjectScriptMember(class, offset)
		if !ok {
			close := strings.IndexByte(class[offset:], '>')
			if close < 0 {
				break
			}
			offset += close + 1
			continue
		}
		switch kind {
		case "Method":
			emitObjectScriptMethod(&output, tag)
		case "Property":
			emitObjectScriptProperty(&output, tag)
		case "Parameter":
			emitObjectScriptParameter(&output, tag)
		case "Index":
			emitObjectScriptIndex(&output, tag)
		case "XData":
			emitObjectScriptXData(&output, tag)
		}
		offset = end
	}
	output.add("}\n")
	return output.value.String()
}

func nextObjectScriptMember(source string, offset int) (string, string, int, bool) {
	for _, kind := range []string{"Method", "Property", "Parameter", "Index", "XData"} {
		prefix := "<" + kind
		if !strings.HasPrefix(source[offset:], prefix+" ") && !strings.HasPrefix(source[offset:], prefix+">") {
			continue
		}
		openEnd := strings.IndexByte(source[offset:], '>')
		if openEnd < 0 {
			return "", "", offset, false
		}
		openEnd += offset
		if isSelfClosingXML(source[offset : openEnd+1]) {
			return source[offset : openEnd+1], kind, openEnd + 1, true
		}
		closing := "</" + kind + ">"
		closeAt := strings.Index(source[openEnd+1:], closing)
		if closeAt < 0 {
			return "", "", openEnd + 1, false
		}
		closeAt += openEnd + 1
		end := closeAt + len(closing)
		return source[offset:end], kind, end, true
	}
	return "", "", offset, false
}

func emitObjectScriptMethod(output *boundedUDL, member string) {
	name := xmlOpenAttribute(member, "name", 255)
	if name == "" {
		return
	}
	if description, ok := xmlContent(member, "Description", 4095); ok && description != "" {
		output.add("/// ")
		output.add(strings.ReplaceAll(description, "\n", "\n/// "))
		output.add("\n")
	}
	if xmlTagIsOne(member, "ClassMethod") {
		output.add("ClassMethod ")
	} else {
		output.add("Method ")
	}
	output.add(name)
	output.add("(")
	if formal, ok := xmlContent(member, "FormalSpec", 1023); ok {
		output.add(formal)
	}
	output.add(")")
	if returnType, ok := xmlContent(member, "ReturnType", 255); ok && returnType != "" {
		output.add(" As ")
		output.add(returnType)
	}
	output.add("\n{\n")
	if implementation, ok := xmlContent(member, "Implementation", (32<<10)-1); ok {
		output.add(implementation)
	}
	output.add("}\n\n")
}

func emitObjectScriptProperty(output *boundedUDL, member string) {
	name := xmlOpenAttribute(member, "name", 255)
	if name == "" {
		return
	}
	output.add("Property ")
	output.add(name)
	if propertyType, ok := xmlContent(member, "Type", 255); ok && propertyType != "" {
		output.add(" As ")
		output.add(propertyType)
	}
	parameters := [][2]string{}
	for rest := member; len(parameters) < 32; {
		start := strings.Index(rest, "<Parameter ")
		if start < 0 {
			break
		}
		rest = rest[start:]
		end := strings.IndexByte(rest, '>')
		if end < 0 {
			break
		}
		name := xmlAttribute(rest[:end], "name", 255)
		value := xmlAttribute(rest[:end], "value", 255)
		if name != "" {
			parameters = append(parameters, [2]string{name, value})
		}
		rest = rest[end+1:]
	}
	if len(parameters) > 0 {
		output.add("(")
		for index, parameter := range parameters {
			if index > 0 {
				output.add(", ")
			}
			output.add(parameter[0])
			if parameter[1] != "" {
				output.add(" = ")
				output.add(parameter[1])
			}
		}
		output.add(")")
	}
	output.add(";\n\n")
}

func emitObjectScriptParameter(output *boundedUDL, member string) {
	name := xmlOpenAttribute(member, "name", 255)
	if name == "" {
		return
	}
	output.add("Parameter ")
	output.add(name)
	if value, ok := xmlContent(member, "Default", 255); ok && value != "" {
		output.add(" = \"")
		output.add(value)
		output.add("\"")
	}
	output.add(";\n\n")
}

func emitObjectScriptIndex(output *boundedUDL, member string) {
	name := xmlOpenAttribute(member, "name", 255)
	if name == "" {
		return
	}
	output.add("Index ")
	output.add(name)
	if properties, ok := xmlContent(member, "Properties", 1023); ok && properties != "" {
		output.add(" On ")
		output.add(properties)
	}
	primary, unique := xmlTagIsOne(member, "PrimaryKey"), xmlTagIsOne(member, "Unique")
	if primary || unique {
		output.add(" [ ")
		if primary {
			output.add("PrimaryKey, ")
		}
		if unique {
			output.add("Unique")
		}
		output.add(" ]")
	}
	output.add(";\n\n")
}

func emitObjectScriptXData(output *boundedUDL, member string) {
	name := xmlOpenAttribute(member, "name", 255)
	if name == "" {
		return
	}
	output.add("XData ")
	output.add(name)
	output.add("\n{\n")
	if data, ok := xmlContent(member, "Data", (32<<10)-1); ok {
		output.add(data)
	}
	output.add("\n}\n\n")
}

func xmlOpenAttribute(element, attribute string, limit int) string {
	end := strings.IndexByte(element, '>')
	if end < 0 {
		return ""
	}
	return xmlAttribute(element[:end], attribute, limit)
}

func xmlAttribute(tag, attribute string, limit int) string {
	needle := attribute + "="
	for offset := 0; ; {
		index := strings.Index(tag[offset:], needle)
		if index < 0 {
			return ""
		}
		index += offset
		start := index + len(needle)
		if start < len(tag) && (tag[start] == '\'' || tag[start] == '"') {
			quote := tag[start]
			end := strings.IndexByte(tag[start+1:], quote)
			if end < 0 {
				return ""
			}
			value := tag[start+1 : start+1+end]
			if len(value) > limit {
				value = value[:limit]
			}
			return value
		}
		offset = start
	}
}

func xmlContent(source, tag string, limit int) (string, bool) {
	start := strings.Index(source, "<"+tag)
	if start < 0 {
		return "", false
	}
	openEnd := strings.IndexByte(source[start:], '>')
	if openEnd < 0 {
		return "", false
	}
	openEnd += start
	if isSelfClosingXML(source[start : openEnd+1]) {
		return "", true
	}
	contentStart := openEnd + 1
	var value string
	if strings.HasPrefix(source[contentStart:], "<![CDATA[") {
		contentStart += len("<![CDATA[")
		end := strings.Index(source[contentStart:], "]]>")
		if end < 0 {
			return "", false
		}
		value = source[contentStart : contentStart+end]
	} else {
		end := strings.Index(source[contentStart:], "</"+tag+">")
		if end < 0 {
			return "", false
		}
		value = source[contentStart : contentStart+end]
	}
	if len(value) > limit {
		value = value[:limit]
	}
	return value, true
}

func xmlTagIsOne(source, tag string) bool {
	value, ok := xmlContent(source, tag, 7)
	return ok && value == "1"
}

func isSelfClosingXML(tag string) bool {
	return strings.HasSuffix(strings.TrimSpace(strings.TrimSuffix(tag, ">")), "/")
}
