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

	"github.com/TheGeeKing/FileGot/internal/mediainfo"
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
	Technical    map[string]string
	Media        map[string]string
	Video        []map[string]string
	Audio        []map[string]string
	Text         []map[string]string
	Chapters     map[string]string
	Image        map[string]string
	Menu         map[string]string
}

type AdvancedTemplateParameterType uint8

const (
	AdvancedTemplateString AdvancedTemplateParameterType = iota
	AdvancedTemplateInteger
	AdvancedTemplateRegularExpression
)

func (parameterType AdvancedTemplateParameterType) String() string {
	switch parameterType {
	case AdvancedTemplateInteger:
		return "Integer"
	case AdvancedTemplateRegularExpression:
		return "Regular expression"
	default:
		return "String"
	}
}

type AdvancedTemplateParameter struct {
	Name        string
	Type        AdvancedTemplateParameterType
	Required    bool
	Description string
}

type AdvancedTemplateSyntax struct {
	Name        string
	Syntax      string
	Description string
	Example     string
	ReturnType  string
	Parameters  []AdvancedTemplateParameter
}

type AdvancedTemplateCompletion struct {
	AdvancedTemplateSyntax
	InsertText   string
	ReplaceStart int
	ReplaceEnd   int
	CursorBack   int
}

type AdvancedTemplateSignature struct {
	AdvancedTemplateSyntax
	ActiveParameter int
}

type fileBotBinding struct {
	AdvancedTemplateSyntax
	field    string
	key      string
	metadata bool
}

var fileBotBindings = map[Kind][]fileBotBinding{
	Movie: {
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "n", Syntax: "{n}", Description: "Movie title",
				Example: "Dune: Part Two", ReturnType: "String",
			},
			field: "Name",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "ny", Syntax: "{ny}", Description: "Movie title and release year",
				Example: "Dune: Part Two (2024)", ReturnType: "String",
			},
			field: "NameYear",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "primaryTitle", Syntax: "{primaryTitle}", Description: "Original movie title",
				Example: "Dune: Part Two", ReturnType: "String",
			},
			field: "PrimaryTitle",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "tmdbid", Syntax: "{tmdbid}", Description: "TMDB movie ID",
				Example: "438631", ReturnType: "Integer",
			},
			field: "TMDBID",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "y", Syntax: "{y}", Description: "Release year",
				Example: "2024", ReturnType: "Integer",
			},
			field: "Year",
		},
	},
	Episode: {
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "e", Syntax: "{e}", Description: "Episode number",
				Example: "3", ReturnType: "Integer",
			},
			field: "Episode",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "n", Syntax: "{n}", Description: "Series title",
				Example: "The Last of Us", ReturnType: "String",
			},
			field: "Name",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "ny", Syntax: "{ny}", Description: "Series title and premiere year",
				Example: "The Last of Us (2023)", ReturnType: "String",
			},
			field: "NameYear",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "primaryTitle", Syntax: "{primaryTitle}", Description: "Original series title",
				Example: "The Last of Us", ReturnType: "String",
			},
			field: "PrimaryTitle",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "s", Syntax: "{s}", Description: "Season number",
				Example: "1", ReturnType: "Integer",
			},
			field: "Season",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "s00e00", Syntax: "{s00e00}", Description: "Padded season and episode",
				Example: "S01E03", ReturnType: "String",
			},
			field: "S00E00",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "sxe", Syntax: "{sxe}", Description: "Season and episode",
				Example: "1x03", ReturnType: "String",
			},
			field: "SxE",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "t", Syntax: "{t}", Description: "Episode title",
				Example: "Long, Long Time", ReturnType: "String",
			},
			field: "EpisodeTitle",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "tmdbid", Syntax: "{tmdbid}", Description: "TMDB series ID",
				Example: "100088", ReturnType: "Integer",
			},
			field: "TMDBID",
		},
		{
			AdvancedTemplateSyntax: AdvancedTemplateSyntax{
				Name: "y", Syntax: "{y}", Description: "Series premiere year",
				Example: "2023", ReturnType: "Integer",
			},
			field: "Year",
		},
	},
}

var technicalBindingNames = []string{
	"vcf", "vc", "ac", "cf", "vf", "hpi", "vk", "aco", "acf", "af", "channels",
	"resolution", "width", "height", "bitdepth", "hdr", "dovi", "bitrate", "vbr",
	"abr", "fps", "khz", "ar", "ws", "hd", "s3d", "mediaTitle", "audioLanguages",
	"textLanguages", "duration", "seconds", "minutes", "hours",
}

func AdvancedTemplateUsesTechnicalMetadata(pattern string) bool {
	for _, binding := range fileBotBindings[Movie] {
		if binding.metadata && regexp.MustCompile(`\b`+regexp.QuoteMeta(binding.Name)+`\b`).MatchString(pattern) {
			return true
		}
	}
	return false
}

var rawBindingDefinitions = []struct {
	name, field, returnType string
}{
	{"media", "Media", "Map"}, {"video", "Video", "List"}, {"audio", "Audio", "List"},
	{"text", "Text", "List"}, {"chapters", "Chapters", "Map"},
	{"image", "Image", "Map"}, {"menu", "Menu", "Map"},
}

var metadataFields = map[string]bool{"Technical": true}

func init() {
	for kind := range fileBotBindings {
		for _, name := range technicalBindingNames {
			fileBotBindings[kind] = append(fileBotBindings[kind], fileBotBinding{
				AdvancedTemplateSyntax: AdvancedTemplateSyntax{
					Name: name, Syntax: "{" + name + "}", Description: "Technical media metadata",
					Example: technicalExample(name), ReturnType: "String",
				},
				field: "Technical", key: name, metadata: true,
			})
		}
		for _, raw := range rawBindingDefinitions {
			metadataFields[raw.field] = true
			fileBotBindings[kind] = append(fileBotBindings[kind], fileBotBinding{
				AdvancedTemplateSyntax: AdvancedTemplateSyntax{
					Name: raw.name, Syntax: "{" + raw.name + "}", Description: "Raw MediaInfo object",
					Example: "<properties>", ReturnType: raw.returnType,
				},
				field: raw.field, metadata: true,
			})
		}
	}
}

func technicalExample(name string) string {
	if value := map[string]string{
		"vcf": "HEVC", "vc": "x265", "ac": "truehd", "cf": "mkv", "vf": "2160p",
		"resolution": "3840x2160", "hdr": "HDR10", "dovi": "Dolby Vision",
		"aco": "TrueHD+Atmos", "channels": "7.1", "audioLanguages": "en",
	}[name]; value != "" {
		return value
	}
	return "<value>"
}

var advancedTemplateExpressions = []AdvancedTemplateSyntax{
	{
		Name: "Optional fragment", Syntax: `{" ($y)"}`,
		Description: "Omitted when a binding is unavailable", Example: "(2024)", ReturnType: "Expression",
	},
	{
		Name: "Interpolation", Syntax: `$y or ${y}`,
		Description: "Insert a binding inside a quoted fragment", Example: "2024", ReturnType: "Interpolation",
	},
	{
		Name: "Method chain", Syntax: `{n.space('.').lower()}`,
		Description: "Apply methods from left to right", Example: "dune:.part.two", ReturnType: "Method chain",
	},
	{
		Name: "Conditional", Syntax: `{y ? " ($y)" : ""}`,
		Description: "Choose a value using a condition", Example: "(2024)", ReturnType: "Conditional",
	},
	{
		Name: "Conditional operators", Syntax: `!  &&  ||  ==  !=`,
		Description: "Supported conditional operators", Example: `{y != "" && n != "" ? n : ""}`, ReturnType: "Operator",
	},
	{
		Name: "String literal", Syntax: `'text' or "text"`,
		Description: "String argument", Example: "text", ReturnType: "String literal",
	},
	{
		Name: "Regular expression", Syntax: `/pattern/`,
		Description: "RE2 regular-expression argument", Example: `[!?.]+$`, ReturnType: "Regex literal",
	},
	{
		Name: "Integer literal", Syntax: `3`,
		Description: "Positive integer argument", Example: `{y.pad(6)}`, ReturnType: "Integer literal",
	},
}

type fileBotMethod struct {
	AdvancedTemplateSyntax
	function        any
	regexArgument   bool
	allowsMissing   bool
	integerReceiver bool
}

var fileBotMethods = []fileBotMethod{
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "acronym", Syntax: `{n.acronym()}`, Description: "Keep the first letter of each word",
			Example: "DPT", ReturnType: "String",
		},
		function: acronym,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "after", Syntax: `{n.after(': ')}`, Description: "Keep text after a separator",
			Example: "Part Two", ReturnType: "String",
			Parameters: []AdvancedTemplateParameter{
				{Name: "separator", Type: AdvancedTemplateString, Required: true, Description: "Text that marks where the result starts."},
			},
		},
		function: after,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "before", Syntax: `{n.before(': ')}`, Description: "Keep text before a separator",
			Example: "Dune", ReturnType: "String",
			Parameters: []AdvancedTemplateParameter{
				{Name: "separator", Type: AdvancedTemplateString, Required: true, Description: "Text that marks where the result ends."},
			},
		},
		function: before,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "clean", Syntax: `{n.clean()}`, Description: "Remove characters unsafe in filenames",
			Example: "Dune Part Two", ReturnType: "String",
		},
		function: func(value string) string { return Sanitize(value, 0) },
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "colon", Syntax: `{n.colon(' - ')}`, Description: "Replace colons",
			Example: "Dune - Part Two", ReturnType: "String",
			Parameters: []AdvancedTemplateParameter{
				{Name: "replacement", Type: AdvancedTemplateString, Required: true, Description: "Text that replaces each colon."},
			},
		},
		function: func(replacement, value string) string { return strings.ReplaceAll(value, ":", replacement) },
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "default", Syntax: `{primaryTitle.default('Unknown')}`, Description: "Use a fallback when a binding is unavailable",
			Example: "Unknown", ReturnType: "String",
			Parameters: []AdvancedTemplateParameter{
				{Name: "fallback", Type: AdvancedTemplateString, Required: true, Description: "Value used when the binding is unavailable."},
			},
		},
		function:      func(fallback, value string) string { return firstNonEmpty(value, fallback) },
		allowsMissing: true, integerReceiver: true,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "initialName", Syntax: `{n.initialName()}`, Description: "Abbreviate all but the last word",
			Example: "D. P. Two", ReturnType: "String",
		},
		function: initialName,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "lower", Syntax: `{n.lower()}`, Description: "Convert text to lowercase",
			Example: "dune: part two", ReturnType: "String",
		},
		function: strings.ToLower,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "lowerTrail", Syntax: `{n.lowerTrail()}`, Description: "Lowercase every letter except each word's first",
			Example: "Dune: Part Two", ReturnType: "String",
		},
		function: lowerTrail,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "pad", Syntax: `{n.pad(20, '0')}`, Description: "Pad text on the left to a length",
			Example: "000000Dune: Part Two", ReturnType: "String",
			Parameters: []AdvancedTemplateParameter{
				{Name: "length", Type: AdvancedTemplateInteger, Required: true, Description: "Minimum result length."},
				{Name: "padding", Type: AdvancedTemplateString, Description: `Text used for padding; defaults to "0".`},
			},
		},
		function: pad, integerReceiver: true,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "removeAll", Syntax: `{n.removeAll(/[!?.]+$/)}`, Description: "Remove every regular-expression match",
			Example: "Dune: Part Two", ReturnType: "String",
			Parameters: []AdvancedTemplateParameter{
				{Name: "pattern", Type: AdvancedTemplateRegularExpression, Required: true, Description: "RE2 pattern to remove."},
			},
		},
		function: removeAll, regexArgument: true, integerReceiver: true,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "replace", Syntax: `{n.replace('Two', '2')}`, Description: "Replace literal text",
			Example: "Dune: Part 2", ReturnType: "String",
			Parameters: []AdvancedTemplateParameter{
				{Name: "old", Type: AdvancedTemplateString, Required: true, Description: "Text to find."},
				{Name: "new", Type: AdvancedTemplateString, Required: true, Description: "Replacement text."},
			},
		},
		function:        func(old, new, value string) string { return strings.ReplaceAll(value, old, new) },
		integerReceiver: true,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "replaceAll", Syntax: `{n.replaceAll(/Part/, 'Chapter')}`, Description: "Replace every regular-expression match",
			Example: "Dune: Chapter Two", ReturnType: "String",
			Parameters: []AdvancedTemplateParameter{
				{Name: "pattern", Type: AdvancedTemplateRegularExpression, Required: true, Description: "RE2 pattern to find."},
				{Name: "replacement", Type: AdvancedTemplateString, Required: true, Description: "Replacement text."},
			},
		},
		function: replaceAll, regexArgument: true, integerReceiver: true,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "roman", Syntax: `{n.roman()}`, Description: "Convert standalone numbers from 1 through 12",
			Example: "Episode IV", ReturnType: "String",
		},
		function: roman, integerReceiver: true,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "slash", Syntax: `{n.slash('.')}`, Description: "Replace forward and backward slashes",
			Example: "Dune.Part Two", ReturnType: "String",
			Parameters: []AdvancedTemplateParameter{
				{Name: "replacement", Type: AdvancedTemplateString, Required: true, Description: "Text that replaces each slash."},
			},
		},
		function: replaceSlashes,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "sortName", Syntax: `{n.sortName()}`, Description: "Remove a leading English article",
			Example: "Walking Dead", ReturnType: "String",
		},
		function: sortName,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "space", Syntax: `{n.space('.')}`, Description: "Replace whitespace",
			Example: "Dune:.Part.Two", ReturnType: "String",
			Parameters: []AdvancedTemplateParameter{
				{Name: "replacement", Type: AdvancedTemplateString, Required: true, Description: "Text that replaces each whitespace run."},
			},
		},
		function: func(replacement, value string) string { return whitespace.ReplaceAllString(value, replacement) },
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "trim", Syntax: `{n.trim()}`, Description: "Remove surrounding whitespace",
			Example: "Dune: Part Two", ReturnType: "String",
		},
		function: strings.TrimSpace,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "upper", Syntax: `{n.upper()}`, Description: "Convert text to uppercase",
			Example: "DUNE: PART TWO", ReturnType: "String",
		},
		function: strings.ToUpper,
	},
	{
		AdvancedTemplateSyntax: AdvancedTemplateSyntax{
			Name: "upperInitial", Syntax: `{n.upperInitial()}`, Description: "Uppercase each word's first letter",
			Example: "Dune: Part Two", ReturnType: "String",
		},
		function: upperInitial,
	},
}

var romanNumber = regexp.MustCompile(`\b(?:1[0-2]|[1-9])\b`)

var fileBotFunctions = func() template.FuncMap {
	functions := template.FuncMap{"required": required, "property": property, "at": at}
	for _, method := range fileBotMethods {
		functions[method.Name] = method.function
	}
	return functions
}()

func AdvancedTemplateMethods() []string {
	methods := make([]string, 0, len(fileBotMethods))
	for _, method := range fileBotMethods {
		methods = append(methods, method.Name)
	}
	sort.Strings(methods)
	return methods
}

func AdvancedTemplateCatalog(kind Kind) []AdvancedTemplateSyntax {
	bindings := fileBotBindings[kind]
	catalog := make([]AdvancedTemplateSyntax, 0, len(bindings)+len(advancedTemplateExpressions)+len(fileBotMethods))
	for _, binding := range bindings {
		catalog = append(catalog, binding.AdvancedTemplateSyntax)
	}
	catalog = append(catalog, advancedTemplateExpressions...)
	for _, method := range fileBotMethods {
		catalog = append(catalog, method.AdvancedTemplateSyntax)
	}
	return catalog
}

func AdvancedTemplateCompletions(kind Kind, pattern string, cursor int) []AdvancedTemplateCompletion {
	runes := []rune(pattern)
	start, inLiteral := advancedExpressionAt(runes, cursor)
	if start < 0 {
		return nil
	}
	if inLiteral {
		if !advancedOuterString(runes[start+1 : cursor]) {
			return nil
		}
		return advancedInterpolationCompletions(kind, runes, start, cursor)
	}

	expression := runes[start+1 : cursor]
	prefixStart := 0
	for prefixStart < len(expression) && unicode.IsSpace(expression[prefixStart]) {
		prefixStart++
	}
	if prefix := expression[prefixStart:]; advancedIdentifier(prefix) {
		return advancedCompletions(
			fileBotBindingSyntax(kind),
			string(prefix),
			start+1+prefixStart,
			advancedIdentifierEnd(runes, cursor),
			"",
		)
	}

	dot := advancedTopLevelDot(expression)
	if dot < 0 || !advancedIdentifier(expression[dot+1:]) {
		return nil
	}
	base := strings.TrimSpace(string(expression[:dot]))
	normalized, err := normalizeFileBotExpression(base)
	if err != nil {
		return nil
	}
	node, err := parser.ParseExpr(normalized)
	if err != nil {
		return nil
	}
	_, _, _, receiverType, _, err := compileFileBotNode(kind, node)
	if err != nil {
		return nil
	}
	return advancedCompletions(
		fileBotMethodSyntax(receiverType),
		string(expression[dot+1:]),
		start+1+dot+1,
		advancedIdentifierEnd(runes, cursor),
		base,
	)
}

func advancedInterpolationCompletions(kind Kind, pattern []rune, start, cursor int) []AdvancedTemplateCompletion {
	for dollar := cursor - 1; dollar > start; dollar-- {
		if pattern[dollar] != '$' {
			continue
		}
		expression := pattern[dollar+1 : cursor]
		dot := advancedTopLevelDot(expression)
		if dot < 0 || !advancedIdentifier(expression[dot+1:]) {
			continue
		}
		base := strings.TrimSpace(string(expression[:dot]))
		normalized, err := normalizeFileBotExpression(base)
		if err != nil {
			continue
		}
		node, err := parser.ParseExpr(normalized)
		if err != nil {
			continue
		}
		_, _, _, receiverType, _, err := compileFileBotNode(kind, node)
		if err != nil {
			continue
		}
		completions := advancedCompletions(
			fileBotMethodSyntax(receiverType),
			string(expression[dot+1:]),
			dollar+1+dot+1,
			advancedIdentifierEnd(pattern, cursor),
			base,
		)
		for index := range completions {
			completions[index].Syntax = `{"$` +
				strings.TrimSuffix(strings.TrimPrefix(completions[index].Syntax, "{"), "}") +
				`"}`
		}
		return completions
	}
	return nil
}

type advancedLiteralState struct {
	quote   rune
	regex   bool
	escaped bool
}

func (state *advancedLiteralState) consume(character rune) bool {
	if state.escaped {
		state.escaped = false
		return true
	}
	if state.quote != 0 {
		if character == '\\' {
			state.escaped = true
		} else if character == state.quote {
			state.quote = 0
		}
		return true
	}
	if state.regex {
		if character == '\\' {
			state.escaped = true
		} else if character == '/' {
			state.regex = false
		}
		return true
	}
	switch character {
	case '\'', '"':
		state.quote = character
		return true
	case '/':
		state.regex = true
		return true
	default:
		return false
	}
}

func (state advancedLiteralState) active() bool {
	return state.quote != 0 || state.regex
}

func AdvancedTemplateSignatureHelp(kind Kind, pattern string, cursor int) *AdvancedTemplateSignature {
	runes := []rune(pattern)
	start, inLiteral := advancedExpressionAt(runes, cursor)
	if start < 0 {
		return nil
	}

	type call struct {
		name     string
		receiver string
		commas   int
	}
	var calls []call
	var literal advancedLiteralState
	expression := runes[start+1 : cursor]
	if inLiteral && advancedOuterString(expression) {
		dollar := -1
		for index := len(expression) - 1; index >= 0; index-- {
			if expression[index] == '$' {
				dollar = index
				break
			}
		}
		if dollar < 0 {
			return nil
		}
		expression = expression[dollar+1:]
	}
	for index, character := range expression {
		if literal.consume(character) {
			continue
		}
		switch character {
		case '(':
			name, receiver := advancedMethodBefore(expression, index)
			calls = append(calls, call{name: name, receiver: receiver})
		case ')':
			if len(calls) > 0 {
				calls = calls[:len(calls)-1]
			}
		case ',':
			if len(calls) > 0 {
				calls[len(calls)-1].commas++
			}
		}
	}

	for index := len(calls) - 1; index >= 0; index-- {
		method, ok := fileBotMethodByName(calls[index].name)
		if !ok {
			continue
		}
		tooManyArguments := len(method.Parameters) == 0 && calls[index].commas > 0 ||
			len(method.Parameters) > 0 && calls[index].commas >= len(method.Parameters)
		receiverType, ok := advancedReceiverType(kind, calls[index].receiver)
		if !ok || !method.acceptsReceiver(receiverType) || tooManyArguments {
			return nil
		}
		active := calls[index].commas
		return &AdvancedTemplateSignature{
			AdvancedTemplateSyntax: method.AdvancedTemplateSyntax,
			ActiveParameter:        active,
		}
	}
	return nil
}

func advancedOuterString(expression []rune) bool {
	for _, character := range expression {
		if unicode.IsSpace(character) {
			continue
		}
		return character == '\'' || character == '"'
	}
	return false
}

func advancedExpressionAt(pattern []rune, cursor int) (int, bool) {
	if cursor < 0 || cursor > len(pattern) {
		return -1, false
	}
	start := -1
	var literal advancedLiteralState
	for index, character := range pattern[:cursor] {
		if start < 0 {
			if character == '{' {
				start = index
			}
			continue
		}
		if literal.consume(character) {
			continue
		}
		if character == '}' {
			start = -1
		}
	}
	return start, literal.active()
}

func advancedTopLevelDot(expression []rune) int {
	dot, depth := -1, 0
	var literal advancedLiteralState
	for index, character := range expression {
		if literal.consume(character) {
			continue
		}
		switch character {
		case '(':
			depth++
		case ')':
			depth--
		case '.':
			if depth == 0 {
				dot = index
			}
		}
	}
	return dot
}

func advancedMethodBefore(expression []rune, opening int) (string, string) {
	end := opening
	for end > 0 && unicode.IsSpace(expression[end-1]) {
		end--
	}
	start := end
	for start > 0 && advancedIdentifierRune(expression[start-1]) {
		start--
	}
	dot := start
	for dot > 0 && unicode.IsSpace(expression[dot-1]) {
		dot--
	}
	if start == end || dot == 0 || expression[dot-1] != '.' {
		return "", ""
	}
	return string(expression[start:end]), strings.TrimSpace(string(expression[:dot-1]))
}

func advancedReceiverType(kind Kind, receiver string) (string, bool) {
	normalized, err := normalizeFileBotExpression(receiver)
	if err != nil {
		return "", false
	}
	node, err := parser.ParseExpr(normalized)
	if err != nil {
		return "", false
	}
	_, _, _, receiverType, _, err := compileFileBotNode(kind, node)
	if err != nil {
		return "", false
	}
	return receiverType, true
}

func advancedIdentifier(value []rune) bool {
	for _, character := range value {
		if !advancedIdentifierRune(character) {
			return false
		}
	}
	return true
}

func advancedIdentifierRune(character rune) bool {
	return character == '_' || unicode.IsLetter(character) || unicode.IsDigit(character)
}

func advancedIdentifierEnd(pattern []rune, cursor int) int {
	for cursor < len(pattern) && advancedIdentifierRune(pattern[cursor]) {
		cursor++
	}
	return cursor
}

func advancedCompletions(
	syntax []AdvancedTemplateSyntax,
	prefix string,
	start, end int,
	receiver string,
) []AdvancedTemplateCompletion {
	var completions []AdvancedTemplateCompletion
	for _, item := range syntax {
		if !strings.HasPrefix(item.Name, prefix) {
			continue
		}
		insert := item.Name
		cursorBack := 0
		if receiver != "" {
			insert += "()"
			if len(item.Parameters) > 0 {
				cursorBack = 1
			}
			if dot := strings.Index(item.Syntax, "."); dot >= 0 {
				item.Syntax = "{" + receiver + item.Syntax[dot:]
			}
		}
		completions = append(completions, AdvancedTemplateCompletion{
			AdvancedTemplateSyntax: item,
			InsertText:             insert,
			ReplaceStart:           start,
			ReplaceEnd:             end,
			CursorBack:             cursorBack,
		})
	}
	return completions
}

func fileBotBindingSyntax(kind Kind) []AdvancedTemplateSyntax {
	bindings := fileBotBindings[kind]
	syntax := make([]AdvancedTemplateSyntax, len(bindings))
	for index, binding := range bindings {
		syntax[index] = binding.AdvancedTemplateSyntax
	}
	return syntax
}

func fileBotMethodSyntax(receiverType string) []AdvancedTemplateSyntax {
	var syntax []AdvancedTemplateSyntax
	for _, method := range fileBotMethods {
		if method.acceptsReceiver(receiverType) {
			syntax = append(syntax, method.AdvancedTemplateSyntax)
		}
	}
	return syntax
}

func fileBotBindingByName(kind Kind, name string) (fileBotBinding, bool) {
	for _, binding := range fileBotBindings[kind] {
		if binding.Name == name {
			return binding, true
		}
	}
	return fileBotBinding{}, false
}

func fileBotMethodByName(name string) (fileBotMethod, bool) {
	for _, method := range fileBotMethods {
		if method.Name == name {
			return method, true
		}
	}
	return fileBotMethod{}, false
}

func (method fileBotMethod) acceptsReceiver(receiverType string) bool {
	return receiverType == "String" || receiverType == "Integer" && method.integerReceiver
}

func ValidateAdvancedPattern(kind Kind, pattern string) error {
	parsed, err := parseAdvanced(kind, pattern)
	if err != nil {
		return err
	}
	_, err = executeAdvanced(parsed, "example.mkv", exampleCandidate(kind), exampleTechnicalMetadata())
	return err
}

func FormatAdvanced(pattern, originalPath string, candidate Candidate) (string, error) {
	return FormatAdvancedWithMetadata(pattern, originalPath, candidate, mediainfo.Metadata{})
}

func FormatAdvancedWithMetadata(
	pattern, originalPath string,
	candidate Candidate,
	technical mediainfo.Metadata,
) (string, error) {
	parsed, err := parseAdvanced(candidate.Kind, pattern)
	if err != nil {
		return "", err
	}
	return executeAdvanced(parsed, originalPath, candidate, technical)
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

func executeAdvanced(
	parsed *template.Template,
	originalPath string,
	candidate Candidate,
	technical mediainfo.Metadata,
) (string, error) {
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
	if err := parsed.Execute(&output, namingData(candidate, technical)); err != nil {
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
	field, binding, pipeline, _, allowsMissing, err := compileFileBotNode(kind, node)
	if err != nil {
		return "", err
	}
	if allowsMissing {
		return "{{." + field + pipeline + "}}", nil
	}
	return requiredTemplate(binding, field, pipeline), nil
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

func compileFileBotNode(kind Kind, node ast.Expr) (string, string, string, string, bool, error) {
	switch current := node.(type) {
	case *ast.Ident:
		binding, ok := fileBotBindingByName(kind, current.Name)
		if !ok {
			return "", "", "", "", false, fmt.Errorf("binding %s is not available for %s names", current.Name, kind)
		}
		pipeline := ""
		if binding.key != "" {
			pipeline = " | property " + strconv.Quote(binding.key)
		}
		return binding.field, current.Name, pipeline, binding.ReturnType, false, nil
	case *ast.IndexExpr:
		field, binding, pipeline, receiverType, allowsMissing, err := compileFileBotNode(kind, current.X)
		if err != nil {
			return "", "", "", "", false, err
		}
		if receiverType == "Map" {
			key, ok := current.Index.(*ast.BasicLit)
			if !ok || key.Kind != token.STRING {
				return "", "", "", "", false, fmt.Errorf("map index must be a string")
			}
			value, err := strconv.Unquote(key.Value)
			if err != nil {
				return "", "", "", "", false, fmt.Errorf("invalid map key")
			}
			return field, binding, pipeline + " | property " + strconv.Quote(value), "String", allowsMissing, nil
		}
		if receiverType != "List" {
			return "", "", "", "", false, fmt.Errorf("only list bindings can be indexed")
		}
		index, ok := current.Index.(*ast.BasicLit)
		if !ok || index.Kind != token.INT || strings.Trim(index.Value, "0123456789") != "" {
			return "", "", "", "", false, fmt.Errorf("list index must be a positive integer")
		}
		return field, binding, pipeline + " | at " + index.Value, "Map", allowsMissing, nil
	case *ast.SelectorExpr:
		field, binding, pipeline, receiverType, allowsMissing, err := compileFileBotNode(kind, current.X)
		if err != nil {
			return "", "", "", "", false, err
		}
		if receiverType != "Map" {
			return "", "", "", "", false, fmt.Errorf("properties require a raw media object")
		}
		return field, binding, pipeline + " | property " + strconv.Quote(current.Sel.Name), "String", allowsMissing, nil
	case *ast.CallExpr:
		selector, ok := current.Fun.(*ast.SelectorExpr)
		if !ok {
			return "", "", "", "", false, fmt.Errorf("only binding methods are allowed")
		}
		field, binding, pipeline, receiverType, allowsMissing, err := compileFileBotNode(kind, selector.X)
		if err != nil {
			return "", "", "", "", false, err
		}
		arguments := make([]fileBotArgument, 0, len(current.Args))
		for _, argument := range current.Args {
			value, err := compileFileBotArgument(argument)
			if err != nil {
				return "", "", "", "", false, err
			}
			arguments = append(arguments, value)
		}
		method := selector.Sel.Name
		if method == "_default" {
			method = "default"
		}
		definition, ok := fileBotMethodByName(method)
		if !ok {
			return "", "", "", "", false, fmt.Errorf("method %s is not allowed", method)
		}
		if !definition.acceptsReceiver(receiverType) {
			return "", "", "", "", false, fmt.Errorf("%s is not available for %s values", method, receiverType)
		}
		step, err := compileFileBotMethod(method, arguments)
		return field, binding, pipeline + step, definition.ReturnType, allowsMissing || definition.allowsMissing, err
	default:
		return "", "", "", "", false, fmt.Errorf("unsupported expression")
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
		field, _, pipeline, _, _, err := compileFileBotNode(kind, current)
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
	field, binding, pipeline, _, allowsMissing, err := compileFileBotNode(kind, node)
	if err != nil {
		return "", fmt.Errorf("conditional results must be strings or binding expressions")
	}
	if allowsMissing {
		return "{{." + field + pipeline + "}}", nil
	}
	return requiredTemplate(binding, field, pipeline), nil
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
		value    string
		field    string
		pipeline string
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
			binding, ok := fileBotBindingByName(kind, name)
			if !ok {
				return "", fmt.Errorf("binding %s is not available for %s names", name, kind)
			}
			parts = append(parts, part{field: binding.field})
			fields = appendUnique(fields, binding.field)
			start = index
			continue
		}
		for index < len(value) && isIdentifierPart(value[index]) {
			index++
		}
		if index == nameStart {
			return "", fmt.Errorf("invalid interpolation")
		}
		index = interpolationExpressionEnd(value, index)
		normalized, err := normalizeFileBotExpression(value[nameStart:index])
		if err != nil {
			return "", err
		}
		node, err := parser.ParseExpr(normalized)
		if err != nil {
			return "", fmt.Errorf("invalid interpolation expression")
		}
		field, _, pipeline, _, _, err := compileFileBotNode(kind, node)
		if err != nil {
			return "", err
		}
		parts = append(parts, part{field: field, pipeline: pipeline})
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
			compiled.WriteString(current.pipeline)
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

func interpolationExpressionEnd(value string, end int) int {
	for end < len(value) && value[end] == '.' {
		nameEnd := end + 1
		for nameEnd < len(value) && isIdentifierPart(value[nameEnd]) {
			nameEnd++
		}
		if nameEnd == end+1 || nameEnd == len(value) || value[nameEnd] != '(' {
			break
		}
		callEnd, ok := interpolationCallEnd(value, nameEnd)
		if !ok {
			return len(value)
		}
		end = callEnd
	}
	return end
}

func interpolationCallEnd(value string, opening int) (int, bool) {
	depth := 0
	var literal advancedLiteralState
	for index, character := range value[opening:] {
		if literal.consume(character) {
			continue
		}
		switch character {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return opening + index + 1, true
			}
		}
	}
	return 0, false
}

type fileBotArgument struct {
	text   string
	number bool
}

func compileFileBotMethod(name string, arguments []fileBotArgument) (string, error) {
	method, ok := fileBotMethodByName(name)
	if !ok {
		return "", fmt.Errorf("method %s is not allowed", name)
	}
	required := 0
	for _, parameter := range method.Parameters {
		if parameter.Required {
			required++
		}
	}
	if len(arguments) < required || len(arguments) > len(method.Parameters) {
		return "", fmt.Errorf("%s expects %d to %d arguments", name, required, len(method.Parameters))
	}
	for index, argument := range arguments {
		integer := method.Parameters[index].Type == AdvancedTemplateInteger
		if integer != argument.number {
			return "", fmt.Errorf("%s argument %s expects %s", name, method.Parameters[index].Name, method.Parameters[index].Type)
		}
	}
	if method.regexArgument {
		if len(arguments) > 0 {
			if _, err := regexp.Compile(arguments[0].text); err != nil {
				return "", fmt.Errorf("%s regular expression: %w", name, err)
			}
		}
	}

	var pipeline strings.Builder
	pipeline.WriteString(" | ")
	pipeline.WriteString(name)
	for _, argument := range arguments {
		pipeline.WriteByte(' ')
		if argument.number {
			pipeline.WriteString(argument.text)
		} else {
			pipeline.WriteString(strconv.Quote(argument.text))
		}
	}
	if name == "pad" && len(arguments) == 1 {
		pipeline.WriteString(` "0"`)
	}
	return pipeline.String(), nil
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

func namingData(candidate Candidate, technical mediainfo.Metadata) templateData {
	year := candidate.Year
	if candidate.Kind == Episode {
		year = candidate.SeriesYear
	}
	data := templateData{
		Name: candidate.Title, Year: number(year),
		TMDBID: number(candidate.ID), PrimaryTitle: candidate.OriginalTitle,
		Technical: technical.Bindings, Media: technical.Media, Video: technical.Video,
		Audio: technical.Audio, Text: technical.Text, Chapters: technical.Chapters,
		Image: technical.Image, Menu: technical.Menu,
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

func exampleTechnicalMetadata() mediainfo.Metadata {
	return mediainfo.Metadata{
		Bindings: map[string]string{
			"vcf": "HEVC", "vc": "x265", "ac": "truehd", "cf": "mkv", "vf": "2160p",
			"hpi": "2160p", "vk": "4K", "aco": "TrueHD+Atmos", "acf": "TrueHD7.1",
			"af": "8ch", "channels": "7.1", "resolution": "3840x2160", "width": "3840",
			"height": "2160", "bitdepth": "10", "hdr": "HDR10", "dovi": "Dolby Vision",
			"bitrate": "18.4 Mbps", "vbr": "16.0 Mbps", "abr": "2.4 Mbps",
			"fps": "23.976 fps", "khz": "48 kHz", "ar": "16∶9", "hd": "UHD",
			"mediaTitle": "Dune: Part Two", "audioLanguages": "en", "textLanguages": "fr",
			"duration": "2h46m12s", "seconds": "9972", "minutes": "166", "hours": "2:46",
		},
		Media: map[string]string{"Format": "Matroska"},
		Video: []map[string]string{{"Format_Profile": "Main 10"}},
		Audio: []map[string]string{{"Format": "MLP FBA"}},
		Text:  []map[string]string{{"Language": "fr"}},
	}
}

func property(name string, values map[string]string) string {
	return values[name]
}

func at(index int, values []map[string]string) map[string]string {
	if index < 0 || index >= len(values) {
		return nil
	}
	return values[index]
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

func requiredTemplate(binding, field, pipeline string) string {
	if metadataFields[field] {
		return "{{required " + strconv.Quote(binding) + " (." + field + pipeline + ")}}"
	}
	return "{{required " + strconv.Quote(binding) + " ." + field + pipeline + "}}"
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
