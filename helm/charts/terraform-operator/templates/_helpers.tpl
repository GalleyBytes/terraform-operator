{{/* vim: set filetype=mustache: */}}

{{/*-=-=- Create chart name and version as used by the chart label =-=-+*/}}

{{- define "_chart.label" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}
