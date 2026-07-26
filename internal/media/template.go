package media

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"
)

type templateData struct {
	Name         string
	Year         string
	NameYear     string
	Season       string
	Episode      string
	SxE          string
	S00E00       string
	EpisodeTitle string
	TMDBID       string
	PrimaryTitle string
}

var fileBotBindings = map[Kind]map[string]string{
	Movie: {
		"n": "Name", "y": "Year", "ny": "NameYear",
		"tmdbid": "TMDBID", "primaryTitle": "PrimaryTitle",
	},
	Episode: {
		"n": "Name", "y": "Year", "ny": "NameYear", "s": "Season", "e": "Episode",
		"sxe": "SxE", "s00e00": "S00E00", "t": "EpisodeTitle",
		"tmdbid": "TMDBID", "primaryTitle": "PrimaryTitle",
	},
}

type methodSignature uint8

const (
	noArguments methodSignature = iota
	oneString
	twoStrings
	padArguments
)

type fileBotMethod struct {
	function      any
	signature     methodSignature
	regexArgument bool
	allowsMissing bool
}

var fileBotMethods = map[string]fileBotMethod{
	"lower":        {function: strings.ToLower, signature: noArguments},
	"upper":        {function: strings.ToUpper, signature: noArguments},
	"trim":         {function: strings.TrimSpace, signature: noArguments},
	"space":        {function: func(replacement, value string) string { return whitespace.ReplaceAllString(value, replacement) }, signature: oneString},
	"pad":          {function: pad, signature: padArguments},
	"replace":      {function: func(old, new, value string) string { return strings.ReplaceAll(value, old, new) }, signature: twoStrings},
	"default":      {function: func(fallback, value string) string { return firstNonEmpty(value, fallback) }, signature: oneString, allowsMissing: true},
	"colon":        {function: func(replacement, value string) string { return strings.ReplaceAll(value, ":", replacement) }, signature: oneString},
	"slash":        {function: replaceSlashes, signature: oneString},
	"before":       {function: before, signature: oneString},
	"after":        {function: after, signature: oneString},
	"removeAll":    {function: removeAll, signature: oneString, regexArgument: true},
	"replaceAll":   {function: replaceAll, signature: twoStrings, regexArgument: true},
	"upperInitial": {function: upperInitial, signature: noArguments},
	"lowerTrail":   {function: lowerTrail, signature: noArguments},
	"sortName":     {function: sortName, signature: noArguments},
	"initialName":  {function: initialName, signature: noArguments},
	"acronym":      {function: acronym, signature: noArguments},
	"roman":        {function: roman, signature: noArguments},
	"clean":        {function: func(value string) string { return Sanitize(value, 0) }, signature: noArguments},
}

var romanNumber = regexp.MustCompile(`\b(?:1[0-2]|[1-9])\b`)

var fileBotFunctions = func() template.FuncMap {
	functions := template.FuncMap{"required": required}
	for name, method := range fileBotMethods {
		functions[name] = method.function
	}
	return functions
}()

func AdvancedTemplateMethods() []string {
	methods := make([]string, 0, len(fileBotMethods))
	for name := range fileBotMethods {
		methods = append(methods, name)
	}
	sort.Strings(methods)
	return methods
}

func ValidateAdvancedPattern(kind Kind, pattern string) error {
	parsed, err := parseAdvanced(kind, pattern)
	if err != nil {
		return err
	}
	_, err = executeAdvanced(parsed, "example.mkv", exampleCandidate(kind))
	return err
}

func FormatAdvanced(pattern, originalPath string, candidate Candidate) (string, error) {
	parsed, err := parseAdvanced(candidate.Kind, pattern)
	if err != nil {
		return "", err
	}
	return executeAdvanced(parsed, originalPath, candidate)
}

func parseAdvanced(kind Kind, pattern string) (*template.Template, error) {
	if _, ok := fileBotBindings[kind]; !ok {
		return nil, fmt.Errorf("unsupported media kind %q", kind)
	}
	if strings.TrimSpace(pattern) == "" {
		return nil, fmt.Errorf("%s template cannot be empty", kind)
	}
	compiled, err := compileFileBotPattern(kind, pattern)
	if err != nil {
		return nil, fmt.Errorf("%s template: %w", kind, err)
	}
	parsed, err := template.New("filename").Funcs(fileBotFunctions).Option("missingkey=error").Parse(compiled)
	if err != nil {
		return nil, fmt.Errorf("%s template: %w", kind, err)
	}
	return parsed, nil
}

func executeAdvanced(parsed *template.Template, originalPath string, candidate Candidate) (string, error) {
	switch candidate.Kind {
	case Movie:
		if strings.TrimSpace(candidate.Title) == "" {
			return "", fmt.Errorf("movie title is required")
		}
	case Episode:
		if strings.TrimSpace(candidate.Title) == "" {
			return "", fmt.Errorf("series title is required")
		}
		if candidate.Episode <= 0 {
			return "", fmt.Errorf("episode number is required")
		}
	default:
		return "", fmt.Errorf("unsupported media kind %q", candidate.Kind)
	}

	var output bytes.Buffer
	if err := parsed.Execute(&output, namingData(candidate)); err != nil {
		return "", fmt.Errorf("%s template: %w", candidate.Kind, err)
	}
	return finishName(output.String(), originalPath)
}

func compileFileBotPattern(kind Kind, pattern string) (string, error) {
	var compiled strings.Builder
	for index := 0; index < len(pattern); {
		if pattern[index] == '}' {
			return "", fmt.Errorf("unexpected } at position %d", index+1)
		}
		if pattern[index] != '{' {
			end := strings.IndexByte(pattern[index:], '{')
			if end < 0 {
				end = len(pattern) - index
			}
			writeTemplateLiteral(&compiled, pattern[index:index+end])
			index += end
			continue
		}

		end, err := fileBotExpressionEnd(pattern, index+1)
		if err != nil {
			return "", err
		}
		expression, err := compileFileBotExpression(kind, pattern[index+1:end])
		if err != nil {
			return "", err
		}
		compiled.WriteString(expression)
		index = end + 1
	}
	return compiled.String(), nil
}

func fileBotExpressionEnd(pattern string, start int) (int, error) {
	var quote byte
	regex := false
	escaped := false
	for index := start; index < len(pattern); index++ {
		character := pattern[index]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		if regex {
			if character == '\\' {
				escaped = true
			} else if character == '/' {
				regex = false
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if character == '/' {
			regex = true
			continue
		}
		if character == '}' {
			return index, nil
		}
	}
	return 0, fmt.Errorf("unclosed expression at position %d", start)
}

func compileFileBotExpression(kind Kind, expression string) (string, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return "", fmt.Errorf("empty expression")
	}
	normalized, err := normalizeFileBotExpression(expression)
	if err != nil {
		return "", err
	}
	node, err := parser.ParseExpr(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid expression %q", expression)
	}
	if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return "", fmt.Errorf("invalid string expression")
		}
		return compileInterpolation(kind, value)
	}
	if call, ok := node.(*ast.CallExpr); ok {
		if function, ok := call.Fun.(*ast.Ident); ok && function.Name == "fileBotIf" {
			return compileFileBotConditional(kind, call.Args)
		}
	}
	field, binding, pipeline, allowsMissing, err := compileFileBotNode(kind, node)
	if err != nil {
		return "", err
	}
	if allowsMissing {
		return "{{." + field + pipeline + "}}", nil
	}
	return "{{required " + strconv.Quote(binding) + " ." + field + pipeline + "}}", nil
}

func normalizeFileBotExpression(expression string) (string, error) {
	condition, whenTrue, whenFalse, conditional, err := splitFileBotConditional(expression)
	if err != nil {
		return "", err
	}
	if conditional {
		condition, err = normalizeFileBotExpression(condition)
		if err != nil {
			return "", err
		}
		whenTrue, err = normalizeFileBotExpression(whenTrue)
		if err != nil {
			return "", err
		}
		whenFalse, err = normalizeFileBotExpression(whenFalse)
		if err != nil {
			return "", err
		}
		return "fileBotIf(" + condition + "," + whenTrue + "," + whenFalse + ")", nil
	}

	var normalized strings.Builder
	for index := 0; index < len(expression); {
		switch expression[index] {
		case '"':
			end, err := quotedEnd(expression, index, '"')
			if err != nil {
				return "", err
			}
			normalized.WriteString(expression[index : end+1])
			index = end + 1
		case '\'':
			value, end, err := readFileBotDelimited(expression, index, '\'')
			if err != nil {
				return "", err
			}
			normalized.WriteString(strconv.Quote(value))
			index = end + 1
		case '/':
			value, end, err := readFileBotDelimited(expression, index, '/')
			if err != nil {
				return "", err
			}
			normalized.WriteString(strconv.Quote(value))
			index = end + 1
		default:
			if strings.HasPrefix(expression[index:], ".default") {
				end := index + len(".default")
				if end == len(expression) || !isIdentifierPart(expression[end]) {
					normalized.WriteString("._default")
					index = end
					continue
				}
			}
			normalized.WriteByte(expression[index])
			index++
		}
	}
	return normalized.String(), nil
}

func splitFileBotConditional(expression string) (string, string, string, bool, error) {
	var quote byte
	regex := false
	escaped := false
	depth := 0
	question := -1
	for index := 0; index < len(expression); index++ {
		character := expression[index]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		if regex {
			if character == '\\' {
				escaped = true
			} else if character == '/' {
				regex = false
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '/':
			regex = true
		case '(':
			depth++
		case ')':
			depth--
		case '?':
			if depth == 0 && question < 0 {
				question = index
			}
		case ':':
			if depth == 0 && question >= 0 {
				condition := strings.TrimSpace(expression[:question])
				whenTrue := strings.TrimSpace(expression[question+1 : index])
				whenFalse := strings.TrimSpace(expression[index+1:])
				if condition == "" || whenTrue == "" || whenFalse == "" {
					return "", "", "", false, fmt.Errorf("conditional requires condition, true value, and false value")
				}
				return condition, whenTrue, whenFalse, true, nil
			}
		}
		if depth < 0 {
			return "", "", "", false, fmt.Errorf("unexpected )")
		}
	}
	if question >= 0 {
		return "", "", "", false, fmt.Errorf("conditional is missing :")
	}
	return "", "", "", false, nil
}

func quotedEnd(value string, start int, delimiter byte) (int, error) {
	escaped := false
	for index := start + 1; index < len(value); index++ {
		if escaped {
			escaped = false
			continue
		}
		if value[index] == '\\' {
			escaped = true
		} else if value[index] == delimiter {
			return index, nil
		}
	}
	return 0, fmt.Errorf("unclosed literal")
}

func readFileBotDelimited(value string, start int, delimiter byte) (string, int, error) {
	var content strings.Builder
	for index := start + 1; index < len(value); index++ {
		character := value[index]
		if character == delimiter {
			return content.String(), index, nil
		}
		if character != '\\' || index+1 == len(value) {
			content.WriteByte(character)
			continue
		}
		next := value[index+1]
		index++
		if delimiter == '/' {
			content.WriteByte('\\')
			content.WriteByte(next)
			continue
		}
		switch next {
		case 'n':
			content.WriteByte('\n')
		case 'r':
			content.WriteByte('\r')
		case 't':
			content.WriteByte('\t')
		case '\\', '\'', '"':
			content.WriteByte(next)
		default:
			content.WriteByte('\\')
			content.WriteByte(next)
		}
	}
	return "", 0, fmt.Errorf("unclosed literal")
}

func compileFileBotNode(kind Kind, node ast.Expr) (string, string, string, bool, error) {
	switch current := node.(type) {
	case *ast.Ident:
		field, ok := fileBotBindings[kind][current.Name]
		if !ok {
			return "", "", "", false, fmt.Errorf("binding %s is not available for %s names", current.Name, kind)
		}
		return field, current.Name, "", false, nil
	case *ast.CallExpr:
		selector, ok := current.Fun.(*ast.SelectorExpr)
		if !ok {
			return "", "", "", false, fmt.Errorf("only binding methods are allowed")
		}
		field, binding, pipeline, allowsMissing, err := compileFileBotNode(kind, selector.X)
		if err != nil {
			return "", "", "", false, err
		}
		arguments := make([]fileBotArgument, 0, len(current.Args))
		for _, argument := range current.Args {
			value, err := compileFileBotArgument(argument)
			if err != nil {
				return "", "", "", false, err
			}
			arguments = append(arguments, value)
		}
		method := selector.Sel.Name
		if method == "_default" {
			method = "default"
		}
		step, err := compileFileBotMethod(method, arguments)
		return field, binding, pipeline + step, allowsMissing || fileBotMethods[method].allowsMissing, err
	default:
		return "", "", "", false, fmt.Errorf("unsupported expression")
	}
}

func compileFileBotConditional(kind Kind, arguments []ast.Expr) (string, error) {
	if len(arguments) != 3 {
		return "", fmt.Errorf("conditional requires condition, true value, and false value")
	}
	condition, err := compileFileBotCondition(kind, arguments[0])
	if err != nil {
		return "", err
	}
	whenTrue, err := compileFileBotResult(kind, arguments[1])
	if err != nil {
		return "", err
	}
	whenFalse, err := compileFileBotResult(kind, arguments[2])
	if err != nil {
		return "", err
	}
	return "{{if " + condition + "}}" + whenTrue + "{{else}}" + whenFalse + "{{end}}", nil
}

func compileFileBotCondition(kind Kind, node ast.Expr) (string, error) {
	switch current := node.(type) {
	case *ast.ParenExpr:
		return compileFileBotCondition(kind, current.X)
	case *ast.Ident, *ast.CallExpr:
		field, _, pipeline, _, err := compileFileBotNode(kind, current)
		if err != nil {
			return "", err
		}
		return "(." + field + pipeline + ")", nil
	case *ast.UnaryExpr:
		if current.Op != token.NOT {
			return "", fmt.Errorf("only ! is allowed in conditions")
		}
		value, err := compileFileBotCondition(kind, current.X)
		return "(not " + value + ")", err
	case *ast.BinaryExpr:
		function := map[token.Token]string{
			token.LAND: "and", token.LOR: "or", token.EQL: "eq", token.NEQ: "ne",
		}[current.Op]
		if function == "" {
			return "", fmt.Errorf("only &&, ||, ==, and != are allowed in conditions")
		}
		left, err := compileFileBotCondition(kind, current.X)
		if err != nil {
			return "", err
		}
		right, err := compileFileBotCondition(kind, current.Y)
		return "(" + function + " " + left + " " + right + ")", err
	case *ast.BasicLit:
		switch current.Kind {
		case token.STRING:
			value, err := strconv.Unquote(current.Value)
			return strconv.Quote(value), err
		case token.INT:
			if strings.Trim(current.Value, "0123456789") != "" {
				return "", fmt.Errorf("condition integers must be decimal")
			}
			return strconv.Quote(current.Value), nil
		}
	}
	return "", fmt.Errorf("unsupported condition")
}

func compileFileBotResult(kind Kind, node ast.Expr) (string, error) {
	if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			return "", err
		}
		return compileInterpolation(kind, value)
	}
	field, binding, pipeline, allowsMissing, err := compileFileBotNode(kind, node)
	if err != nil {
		return "", fmt.Errorf("conditional results must be strings or binding expressions")
	}
	if allowsMissing {
		return "{{." + field + pipeline + "}}", nil
	}
	return "{{required " + strconv.Quote(binding) + " ." + field + pipeline + "}}", nil
}

func compileFileBotArgument(node ast.Expr) (fileBotArgument, error) {
	literal, ok := node.(*ast.BasicLit)
	if !ok {
		return fileBotArgument{}, fmt.Errorf("arguments must be literal strings or positive integers")
	}
	switch literal.Kind {
	case token.STRING:
		value, err := strconv.Unquote(literal.Value)
		return fileBotArgument{text: value}, err
	case token.INT:
		if strings.Trim(literal.Value, "0123456789") != "" {
			return fileBotArgument{}, fmt.Errorf("integer arguments must be decimal")
		}
		return fileBotArgument{text: literal.Value, number: true}, nil
	default:
		return fileBotArgument{}, fmt.Errorf("arguments must be literal strings or positive integers")
	}
}

func compileInterpolation(kind Kind, value string) (string, error) {
	type part struct {
		value string
		field string
	}
	var parts []part
	var fields []string
	start := 0
	for index := 0; index < len(value); {
		if value[index] != '$' {
			index++
			continue
		}
		if index > start {
			parts = append(parts, part{value: value[start:index]})
		}
		index++
		nameStart := index
		if index < len(value) && value[index] == '{' {
			index++
			nameStart = index
			for index < len(value) && value[index] != '}' {
				index++
			}
			if index == len(value) {
				return "", fmt.Errorf("unclosed interpolation")
			}
			name := value[nameStart:index]
			index++
			field, ok := fileBotBindings[kind][name]
			if !ok {
				return "", fmt.Errorf("binding %s is not available for %s names", name, kind)
			}
			parts = append(parts, part{field: field})
			fields = appendUnique(fields, field)
			start = index
			continue
		}
		for index < len(value) && isIdentifierPart(value[index]) {
			index++
		}
		if index == nameStart {
			return "", fmt.Errorf("invalid interpolation")
		}
		name := value[nameStart:index]
		field, ok := fileBotBindings[kind][name]
		if !ok {
			return "", fmt.Errorf("binding %s is not available for %s names", name, kind)
		}
		parts = append(parts, part{field: field})
		fields = appendUnique(fields, field)
		start = index
	}
	if start < len(value) {
		parts = append(parts, part{value: value[start:]})
	}

	var compiled strings.Builder
	if len(fields) == 1 {
		compiled.WriteString("{{if .")
		compiled.WriteString(fields[0])
		compiled.WriteString("}}")
	} else if len(fields) > 1 {
		compiled.WriteString("{{if and")
		for _, field := range fields {
			compiled.WriteString(" .")
			compiled.WriteString(field)
		}
		compiled.WriteString("}}")
	}
	for _, current := range parts {
		if current.field != "" {
			compiled.WriteString("{{.")
			compiled.WriteString(current.field)
			compiled.WriteString("}}")
		} else {
			writeTemplateLiteral(&compiled, current.value)
		}
	}
	if len(fields) > 0 {
		compiled.WriteString("{{end}}")
	}
	return compiled.String(), nil
}

type fileBotArgument struct {
	text   string
	number bool
}

func compileFileBotMethod(name string, arguments []fileBotArgument) (string, error) {
	method, ok := fileBotMethods[name]
	if !ok {
		return "", fmt.Errorf("method %s is not allowed", name)
	}
	if method.regexArgument {
		if len(arguments) > 0 && !arguments[0].number {
			if _, err := regexp.Compile(arguments[0].text); err != nil {
				return "", fmt.Errorf("%s regular expression: %w", name, err)
			}
		}
	}

	switch method.signature {
	case noArguments:
		if len(arguments) != 0 {
			return "", fmt.Errorf("%s expects no arguments", name)
		}
		return " | " + name, nil
	case oneString:
		if len(arguments) != 1 || arguments[0].number {
			return "", fmt.Errorf("%s expects one string argument", name)
		}
		return " | " + name + " " + strconv.Quote(arguments[0].text), nil
	case twoStrings:
		if len(arguments) != 2 || arguments[0].number || arguments[1].number {
			return "", fmt.Errorf("%s expects two string arguments", name)
		}
		return " | " + name + " " + strconv.Quote(arguments[0].text) + " " + strconv.Quote(arguments[1].text), nil
	case padArguments:
		if len(arguments) < 1 || len(arguments) > 2 || !arguments[0].number ||
			(len(arguments) == 2 && arguments[1].number) {
			return "", fmt.Errorf("pad expects a length and optional padding string")
		}
		padding := "0"
		if len(arguments) == 2 {
			padding = arguments[1].text
		}
		return " | pad " + arguments[0].text + " " + strconv.Quote(padding), nil
	}
	return "", fmt.Errorf("method %s has an unsupported signature", name)
}

func writeTemplateLiteral(target *strings.Builder, value string) {
	if value == "" {
		return
	}
	target.WriteString("{{")
	target.WriteString(strconv.Quote(value))
	target.WriteString("}}")
}

func appendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func isIdentifierStart(character byte) bool {
	return character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
}

func isIdentifierPart(character byte) bool {
	return isIdentifierStart(character) || character >= '0' && character <= '9'
}

func namingData(candidate Candidate) templateData {
	year := candidate.Year
	if candidate.Kind == Episode {
		year = candidate.SeriesYear
	}
	data := templateData{
		Name: candidate.Title, Year: number(year),
		TMDBID: number(candidate.ID), PrimaryTitle: candidate.OriginalTitle,
	}
	data.NameYear = data.Name
	if data.Year != "" {
		data.NameYear += " (" + data.Year + ")"
	}
	if candidate.Kind == Episode {
		data.Season = strconv.Itoa(candidate.Season)
		data.Episode = strconv.Itoa(candidate.Episode)
		data.SxE = fmt.Sprintf("%dx%02d", candidate.Season, candidate.Episode)
		data.S00E00 = fmt.Sprintf("S%02dE%02d", candidate.Season, candidate.Episode)
		data.EpisodeTitle = candidate.EpisodeTitle
	}
	return data
}

func exampleCandidate(kind Kind) Candidate {
	if kind == Movie {
		return Candidate{
			ID: 438631, Kind: Movie, Title: "Dune: Part Two",
			OriginalTitle: "Dune: Part Two", Year: 2024,
		}
	}
	return Candidate{
		ID: 100088, Kind: Episode, Title: "The Last of Us",
		OriginalTitle: "The Last of Us", SeriesYear: 2023,
		Season: 1, Episode: 3, EpisodeTitle: "Long, Long Time",
	}
}

func number(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func required(binding, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("binding %s is unavailable", binding)
	}
	return value, nil
}

func pad(length int, padding, value string) string {
	if padding == "" {
		padding = "0"
	}
	current := []rune(value)
	fill := []rune(padding)
	for len(current) < length {
		needed := min(length-len(current), len(fill))
		current = append(fill[:needed], current...)
	}
	return string(current)
}

func replaceSlashes(replacement, value string) string {
	value = strings.ReplaceAll(value, "/", replacement)
	return strings.ReplaceAll(value, `\`, replacement)
}

func before(separator, value string) string {
	if index := strings.Index(value, separator); index >= 0 {
		return value[:index]
	}
	return value
}

func after(separator, value string) string {
	if index := strings.Index(value, separator); index >= 0 {
		return value[index+len(separator):]
	}
	return value
}

func removeAll(pattern, value string) (string, error) {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	return expression.ReplaceAllString(value, ""), nil
}

func replaceAll(pattern, replacement, value string) (string, error) {
	expression, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	return expression.ReplaceAllString(value, replacement), nil
}

func upperInitial(value string) string {
	runes := []rune(value)
	wordStart := true
	for index, character := range runes {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			if wordStart {
				runes[index] = unicode.ToUpper(character)
			}
			wordStart = false
		} else {
			wordStart = true
		}
	}
	return string(runes)
}

func lowerTrail(value string) string {
	runes := []rune(value)
	wordStart := true
	for index, character := range runes {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			if !wordStart {
				runes[index] = unicode.ToLower(character)
			}
			wordStart = false
		} else {
			wordStart = true
		}
	}
	return string(runes)
}

func sortName(value string) string {
	// ponytail: English articles only; add locale-aware articles with locale-aware naming.
	for _, article := range []string{"The", "An", "A"} {
		prefix := article + " "
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return value
}

func initialName(value string) string {
	parts := strings.Fields(value)
	if len(parts) < 2 {
		return value
	}
	for index := range len(parts) - 1 {
		first, _ := utf8FirstRune(parts[index])
		parts[index] = string(first) + "."
	}
	return strings.Join(parts, " ")
}

func acronym(value string) string {
	var result strings.Builder
	for _, part := range strings.Fields(value) {
		first, ok := utf8FirstRune(part)
		if ok && (unicode.IsLetter(first) || unicode.IsNumber(first)) {
			result.WriteRune(unicode.ToUpper(first))
		}
	}
	return result.String()
}

func utf8FirstRune(value string) (rune, bool) {
	for _, character := range value {
		return character, true
	}
	return 0, false
}

func roman(value string) string {
	numbers := map[string]string{
		"1": "I", "2": "II", "3": "III", "4": "IV", "5": "V", "6": "VI",
		"7": "VII", "8": "VIII", "9": "IX", "10": "X", "11": "XI", "12": "XII",
	}
	return romanNumber.ReplaceAllStringFunc(value, func(number string) string { return numbers[number] })
}
