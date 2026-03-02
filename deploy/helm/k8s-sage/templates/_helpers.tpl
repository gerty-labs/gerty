{{/*
Expand the name of the chart.
*/}}
{{- define "k8s-sage.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "k8s-sage.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- printf "%s" $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "k8s-sage.labels" -}}
app.kubernetes.io/name: {{ include "k8s-sage.name" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end }}

{{/*
Agent labels.
*/}}
{{- define "k8s-sage.agent.labels" -}}
{{ include "k8s-sage.labels" . }}
app.kubernetes.io/component: agent
{{- end }}

{{/*
Server labels.
*/}}
{{- define "k8s-sage.server.labels" -}}
{{ include "k8s-sage.labels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Agent selector labels.
*/}}
{{- define "k8s-sage.agent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "k8s-sage.name" . }}
app.kubernetes.io/component: agent
{{- end }}

{{/*
Server selector labels.
*/}}
{{- define "k8s-sage.server.selectorLabels" -}}
app.kubernetes.io/name: {{ include "k8s-sage.name" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Service account name.
*/}}
{{- define "k8s-sage.serviceAccountName" -}}
{{- if .Values.serviceAccount.name }}
{{- .Values.serviceAccount.name }}
{{- else }}
{{- include "k8s-sage.fullname" . }}
{{- end }}
{{- end }}

{{/*
Agent image reference.
*/}}
{{- define "k8s-sage.agent.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- if .Values.image.registry }}
{{- printf "%s/%s:%s" .Values.image.registry .Values.agent.image.repository $tag }}
{{- else }}
{{- printf "%s:%s" .Values.agent.image.repository $tag }}
{{- end }}
{{- end }}

{{/*
Server image reference.
*/}}
{{- define "k8s-sage.server.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- if .Values.image.registry }}
{{- printf "%s/%s:%s" .Values.image.registry .Values.server.image.repository $tag }}
{{- else }}
{{- printf "%s:%s" .Values.server.image.repository $tag }}
{{- end }}
{{- end }}
