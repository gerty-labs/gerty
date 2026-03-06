{{/*
Expand the name of the chart.
*/}}
{{- define "gerty.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "gerty.fullname" -}}
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
{{- define "gerty.labels" -}}
app.kubernetes.io/name: {{ include "gerty.name" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end }}

{{/*
Agent labels.
*/}}
{{- define "gerty.agent.labels" -}}
{{ include "gerty.labels" . }}
app.kubernetes.io/component: agent
{{- end }}

{{/*
Server labels.
*/}}
{{- define "gerty.server.labels" -}}
{{ include "gerty.labels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Agent selector labels.
*/}}
{{- define "gerty.agent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gerty.name" . }}
app.kubernetes.io/component: agent
{{- end }}

{{/*
Server selector labels.
*/}}
{{- define "gerty.server.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gerty.name" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Service account name.
*/}}
{{- define "gerty.serviceAccountName" -}}
{{- if .Values.serviceAccount.name }}
{{- .Values.serviceAccount.name }}
{{- else }}
{{- include "gerty.fullname" . }}
{{- end }}
{{- end }}

{{/*
Agent image reference. Prefers digest over tag when set (marketplace override).
*/}}
{{- define "gerty.agent.image" -}}
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
{{- define "gerty.server.image" -}}
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
{{- define "gerty.slm.image" -}}
{{- if .Values.slm.image.digest }}
{{- printf "%s@%s" .Values.slm.image.repository .Values.slm.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.slm.image.repository .Values.slm.image.tag }}
{{- end }}
{{- end }}

{{/*
Model init container image reference.
Uses modelSize to select the image suffix (e.g., gerty-model-4b, gerty-model-9b).
*/}}
{{- define "gerty.model.image" -}}
{{- $repo := printf "%s-%s" .Values.slm.model.repository .Values.slm.modelSize }}
{{- if .Values.image.registry }}
{{- $repo = printf "%s/%s-%s" .Values.image.registry .Values.slm.model.repository .Values.slm.modelSize }}
{{- end }}
{{- if .Values.slm.model.digest }}
{{- printf "%s@%s" $repo .Values.slm.model.digest }}
{{- else }}
{{- printf "%s:%s" $repo (.Values.slm.model.tag | default .Chart.AppVersion) }}
{{- end }}
{{- end }}
