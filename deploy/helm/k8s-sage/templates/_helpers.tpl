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
Agent image reference. Prefers digest over tag when set (marketplace override).
*/}}
{{- define "k8s-sage.agent.image" -}}
{{- $repo := .Values.agent.image.repository }}
{{- if .Values.image.registry }}
{{- $repo = printf "%s/%s" .Values.image.registry .Values.agent.image.repository }}
{{- end }}
{{- if .Values.agent.image.digest }}
{{- printf "%s@%s" $repo .Values.agent.image.digest }}
{{- else }}
{{- printf "%s:%s" $repo (.Values.image.tag | default .Chart.AppVersion) }}
{{- end }}
{{- end }}

{{/*
Server image reference. Prefers digest over tag when set (marketplace override).
*/}}
{{- define "k8s-sage.server.image" -}}
{{- $repo := .Values.server.image.repository }}
{{- if .Values.image.registry }}
{{- $repo = printf "%s/%s" .Values.image.registry .Values.server.image.repository }}
{{- end }}
{{- if .Values.server.image.digest }}
{{- printf "%s@%s" $repo .Values.server.image.digest }}
{{- else }}
{{- printf "%s:%s" $repo (.Values.image.tag | default .Chart.AppVersion) }}
{{- end }}
{{- end }}

{{/*
SLM image reference. Prefers digest over tag when set (marketplace override).
*/}}
{{- define "k8s-sage.slm.image" -}}
{{- if .Values.slm.image.digest }}
{{- printf "%s@%s" .Values.slm.image.repository .Values.slm.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.slm.image.repository .Values.slm.image.tag }}
{{- end }}
{{- end }}
