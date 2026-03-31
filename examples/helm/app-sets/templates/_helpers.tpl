{{- define "flattenMap" -}}
{{- $prefix := index . 0 -}}
{{- $map := index . 1 -}}
{{- range $key, $value := $map -}}
  {{- $newKey := ternary $key (printf "%s.%s" $prefix $key) (eq $prefix "") -}}
  {{- if kindIs "map" $value }}
{{- include "flattenMap" (list $newKey $value) }}
  {{- else if kindIs "slice" $value -}}
    {{- range $i, $item := $value -}}
      {{- $arrayKey := printf "%s[%d]" $newKey $i -}}
      {{- if kindIs "map" $item }}
{{- include "flattenMap" (list $arrayKey $item) }}
      {{- else }}
{{ $arrayKey }}: {{ $item | quote }}
      {{- end -}}
    {{- end -}}
  {{- else }}
{{ $newKey }}: {{ $value | quote }}
  {{- end -}}
{{- end -}}
{{- end -}}