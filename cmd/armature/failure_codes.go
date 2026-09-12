package main

import armerrors "github.com/scullxbones/armature/internal/errors"

const (
	codeAmend1          = "AMEND-1"
	codeAssign1         = "ASSIGN-1"
	codeBootstrap1      = "BOOTSTRAP-1"
	codeCompletion1     = "COMPLETION-1"
	codeConfirm1        = "CONFIRM-1"
	codeContextHistory1 = "CONTEXT-HISTORY-1"
	codeContextReport1  = "CONTEXT-REPORT-1"
	codeCreate1         = "CREATE-1"
	codeDag1            = "DAG-1"
	codeDecision1       = "DECISION-1"
	codeDoctor1         = "DOCTOR-1"
	codeGate1           = "GATE-1"
	codeHeartbeat1      = "HEARTBEAT-1"
	codeImport1         = "IMPORT-1"
	codeLink1           = "LINK-1"
	codeList1           = "LIST-1"
	codeLog1            = "LOG-1"
	codeMaterialize1    = "MATERIALIZE-1"
	codeMerged1         = "MERGED-1"
	codeNote1           = "NOTE-1"
	codePushOps1        = "PUSH-OPS-1"
	codeReopen1         = "REOPEN-1"
	codeReparent1       = "REPARENT-1"
	codeScopeDelete1    = "SCOPE-DELETE-1"
	codeScopeRename1    = "SCOPE-RENAME-1"
	codeShow1           = "SHOW-1"
	codeSources1        = "SOURCES-1"
	codeStats1          = "STATS-1"
	codeSync1           = "SYNC-1"
	codeTui1            = "TUI-1"
	codeUnassign1       = "UNASSIGN-1"
	codeUnlink1         = "UNLINK-1"
	codeValidate1       = "VALIDATE-1"
	codeVersion1        = "VERSION-1"
	codeWorkerInit1     = "WORKER-INIT-1"
	codeWorkers1        = "WORKERS-1"
	codeWorktree1       = "WORKTREE-1"
)

func init() {
	armerrors.Register(codeAmend1)
	armerrors.Register(codeAssign1)
	armerrors.Register(codeBootstrap1)
	armerrors.Register(codeCompletion1)
	armerrors.Register(codeConfirm1)
	armerrors.Register(codeContextHistory1)
	armerrors.Register(codeContextReport1)
	armerrors.Register(codeCreate1)
	armerrors.Register(codeDag1)
	armerrors.Register(codeDecision1)
	armerrors.Register(codeDoctor1)
	armerrors.Register(codeGate1)
	armerrors.Register(codeHeartbeat1)
	armerrors.Register(codeImport1)
	armerrors.Register(codeLink1)
	armerrors.Register(codeList1)
	armerrors.Register(codeLog1)
	armerrors.Register(codeMaterialize1)
	armerrors.Register(codeMerged1)
	armerrors.Register(codeNote1)
	armerrors.Register(codePushOps1)
	armerrors.Register(codeReopen1)
	armerrors.Register(codeReparent1)
	armerrors.Register(codeScopeDelete1)
	armerrors.Register(codeScopeRename1)
	armerrors.Register(codeShow1)
	armerrors.Register(codeSources1)
	armerrors.Register(codeStats1)
	armerrors.Register(codeSync1)
	armerrors.Register(codeTui1)
	armerrors.Register(codeUnassign1)
	armerrors.Register(codeUnlink1)
	armerrors.Register(codeValidate1)
	armerrors.Register(codeVersion1)
	armerrors.Register(codeWorkerInit1)
	armerrors.Register(codeWorkers1)
	armerrors.Register(codeWorktree1)
}
