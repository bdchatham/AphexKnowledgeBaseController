package controller

import (
	"fmt"

	platformv1alpha1 "github.com/bdchatham/AphexControllerRuntime/api/v1alpha1"
	"github.com/bdchatham/AphexControllerRuntime/pkg/constants"
)

func buildResolveAndTriggerTaskRun(namespace, orgNamespace string, kb *platformv1alpha1.KnowledgeBase) []byte {
	agentParams := ""
	if kb.Spec.Agent != nil && kb.Spec.Agent.APIKeySecretName != "" {
		agentParams = fmt.Sprintf(`\n    - name: agent-api-key-secret\n      value: %s`, kb.Spec.Agent.APIKeySecretName)
		if kb.Spec.Agent.Model != "" {
			agentParams += fmt.Sprintf(`\n    - name: agent-model\n      value: %s`, kb.Spec.Agent.Model)
		}
	}

	return []byte(fmt.Sprintf(`{
  "apiVersion": "tekton.dev/v1",
  "kind": "TaskRun",
  "metadata": {
    "generateName": "scip-sync-resolve-",
    "namespace": %q,
    "labels": {
      "app.kubernetes.io/name": "scip-sync",
      "app.kubernetes.io/component": "resolve"
    }
  },
  "spec": {
    "serviceAccountName": %q,
    "timeout": "5m",
    "taskSpec": {
      "params": [
        {"name": "repo-url", "type": "string"}
      ],
      "steps": [
        {
          "name": "resolve-and-trigger",
          "image": "bitnami/kubectl:latest",
          "script": "#!/bin/bash\nset -euo pipefail\n\nREPO_URL=\"$(params.repo-url)\"\necho \"Resolving KnowledgeBase for repo: ${REPO_URL}\"\n\n# Parse repoOrg_repoName from URL\nREPO_KEY=$(echo \"${REPO_URL}\" | sed 's|https://||;s|http://||;s|\\.git$||' | awk -F/ '{print $(NF-1) \"_\" $NF}')\necho \"Lookup key: ${REPO_KEY}\"\n\nMAPPING=$(kubectl get configmap %s \\\n  -n %s \\\n  -o jsonpath=\"{.data['${REPO_KEY}']}\" 2>/dev/null || true)\n\nif [ -z \"${MAPPING}\" ]; then\n  echo \"No KnowledgeBase mapping found for ${REPO_KEY}, skipping\"\n  exit 0\nfi\n\nKB_NAME=$(echo \"${MAPPING}\" | cut -d'/' -f1)\nKB_NAMESPACE=$(echo \"${MAPPING}\" | cut -d'/' -f2)\necho \"Resolved to KnowledgeBase: ${KB_NAME} in ${KB_NAMESPACE}\"\n\ncat <<EOF | kubectl create -f -\napiVersion: tekton.dev/v1\nkind: TaskRun\nmetadata:\n  generateName: scip-sync-\n  namespace: ${KB_NAMESPACE}\n  labels:\n    app.kubernetes.io/name: scip-sync\n    app.kubernetes.io/component: sync\n    tekton.dev/pipeline: ${KB_NAME}\nspec:\n  serviceAccountName: %s\n  timeout: 1h\n  taskRef:\n    resolver: cluster\n    params:\n      - name: kind\n        value: task\n      - name: name\n        value: scip-sync\n      - name: namespace\n        value: tekton-pipelines\n  params:\n    - name: kb-name\n      value: ${KB_NAME}\n    - name: kb-namespace\n      value: ${KB_NAMESPACE}\n    - name: workspace-name\n      value: ${KB_NAME}%s\n  workspaces:\n    - name: shared-data\n      emptyDir: {}\nEOF\n\necho \"Created scip-sync TaskRun for ${KB_NAME}\""
        }
      ]
    },
    "params": [
      {"name": "repo-url", "value": "$(tt.params.git-url)"}
    ]
  }
}`, namespace, constants.EventTaskResolverServiceAccount, repoMappingConfigMapName, orgNamespace, constants.EventTaskResolverServiceAccount, agentParams))
}
