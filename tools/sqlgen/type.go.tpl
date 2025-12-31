{{- $type := .Type }}
// {{ $type.Name }} represents a row from '{{ $type.Schema }}.{{ $type.SQLName }}'.
type {{ $type.Name }} struct {
{{- range $field := $type.Fields }}
	{{- $dbTag := printf "db:%q" $field.SQLName }}
	{{- if $field.Nullable }}
		{{- $nullableType := "" }}
		{{- if eq $field.Type "string" }}
			{{- $nullableType = "sql.NullString" }}
		{{- else if eq $field.Type "bool" }}
			{{- $nullableType = "sql.NullBool" }}
		{{- else if hasPrefix $field.Type "int" }}
			{{- $nullableType = "sql.NullInt64" }}
		{{- else if hasPrefix $field.Type "float" }}
			{{- $nullableType = "sql.NullFloat64" }}
		{{- else if eq $field.Type "time.Time" }}
			{{- $nullableType = "sql.NullTime" }}
		{{- else }}
			{{- $nullableType = printf "*%s" $field.Type }}
		{{- end }}
	{{ $field.Name }} {{ $nullableType }} `{{ $dbTag }}`{{ if $field.Comment }} // {{ $field.Comment }}{{ end }}
	{{- else }}
	{{ $field.Name }} {{ $field.Type }} `{{ $dbTag }}`{{ if $field.Comment }} // {{ $field.Comment }}{{ end }}
	{{- end }}
{{- end }}
}