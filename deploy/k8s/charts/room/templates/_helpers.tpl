{{- define "room.workload" -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Chart.Name }}
  labels:
    app.kubernetes.io/name: {{ .Chart.Name }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .Chart.Name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{ .Chart.Name }}
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
      terminationGracePeriodSeconds: 20
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          ports:
            - name: grpc
              containerPort: {{ .Values.service.port }}
            - name: health
              containerPort: {{ .Values.service.healthPort }}
          {{- $secrets := .Values.secrets | default list }}
          {{- if or (gt (len .Values.env) 0) (gt (len $secrets) 0) }}
          env:
            {{- range $key, $value := .Values.env }}
            - name: {{ $key }}
              value: {{ $value | quote }}
            {{- end }}
            {{- range $secrets }}
            - name: {{ .name }}
              valueFrom:
                secretKeyRef:
                  name: {{ .secretName }}
                  key: {{ .key }}
            {{- end }}
          {{- end }}
          livenessProbe:
            httpGet:
              path: /healthz
              port: health
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 3
            failureThreshold: 6
          readinessProbe:
            httpGet:
              path: /healthz
              port: health
            periodSeconds: 5
            timeoutSeconds: 3
            failureThreshold: 12
          lifecycle:
            preStop:
              exec:
                command: ["/busybox", "sleep", "5"]
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            runAsNonRoot: true
---
apiVersion: v1
kind: Service
metadata:
  name: {{ .Chart.Name }}
  labels:
    app.kubernetes.io/name: {{ .Chart.Name }}
spec:
  selector:
    app.kubernetes.io/name: {{ .Chart.Name }}
  ports:
    - name: grpc
      port: {{ .Values.service.port }}
      targetPort: grpc
    - name: health
      port: {{ .Values.service.healthPort }}
      targetPort: health
{{- if gt (int .Values.replicaCount) 1 }}
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ .Chart.Name }}
  labels:
    app.kubernetes.io/name: {{ .Chart.Name }}
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .Chart.Name }}
{{- end }}
{{- end }}
