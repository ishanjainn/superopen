package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/ishanjainn/superopen/internal/graph/api"
	"github.com/ishanjainn/superopen/internal/paths"
)

type Server struct {
	EngineVersion string
	Assets        fs.FS
}

func (s Server) Handle(ctx context.Context, req api.Request) api.Response {
	response := api.Response{Protocol: api.ProtocolVersion, RequestID: req.RequestID}
	if req.Protocol != api.ProtocolVersion {
		response.Error = &api.Error{
			Code: "protocol_mismatch", Message: fmt.Sprintf("unsupported protocol %d", req.Protocol),
			Details: map[string]any{"supported": api.ProtocolVersion},
		}
		return response
	}
	result, callErr := s.dispatch(ctx, req)
	if callErr != nil {
		response.Error = callErr
		return response
	}
	body, err := json.Marshal(result)
	if err != nil {
		response.Error = &api.Error{Code: "encode_result", Message: err.Error()}
		return response
	}
	response.OK = true
	response.Result = body
	return response
}

func (s Server) dispatch(ctx context.Context, req api.Request) (any, *api.Error) {
	switch req.Operation {
	case api.OpCapabilities:
		return Capabilities(), nil
	case api.OpBuild:
		var params api.BuildRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		if !paths.Managed(params.RepoRoot) {
			return nil, &api.Error{Code: "not_managed", Message: paths.UnmanagedMessage}
		}
		result, err := IndexAllDevelopment(ctx, params, s.EngineVersion, s.Assets)
		if err != nil {
			return nil, storeError("build", err)
		}
		return result, nil
	case api.OpStatus:
		var params api.StatusRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		paths, err := CachePaths(params.RepoRoot)
		if err != nil {
			return nil, invalidParams(err)
		}
		store, err := OpenReadOnly(paths.Database)
		if err != nil {
			if os.IsNotExist(err) {
				status := api.Status{
					Engine: api.EngineName, EngineVersion: s.EngineVersion,
					Protocol: api.ProtocolVersion, Schema: api.SchemaVersion,
					State: "missing", Database: paths.Database, Capabilities: Capabilities(),
				}
				return status, nil
			}
			return nil, storeError("open_graph", err)
		}
		defer store.Close()
		status, err := store.Status(ctx, params.Project)
		if err != nil {
			return nil, storeError("status", err)
		}
		if status.EngineVersion == "" {
			status.EngineVersion = s.EngineVersion
		}
		return status, nil
	case api.OpSearch:
		var params api.SearchRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		result, err := store.Search(ctx, params)
		if err != nil {
			return nil, storeError("search", err)
		}
		return result, nil
	case api.OpSchema:
		var params api.SchemaRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		result, err := store.Schema(ctx, params.Project)
		if err != nil {
			return nil, storeError("schema", err)
		}
		return result, nil
	case api.OpQuery:
		var params api.QueryRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		result, err := store.Query(ctx, params)
		if err != nil {
			return nil, storeError("query", err)
		}
		RecordQueryStamp(params.RepoRoot)
		return result, nil
	case api.OpCypher:
		var params api.CypherRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		result, err := store.executeCypher(ctx, params)
		if err != nil {
			return nil, storeError("cypher", err)
		}
		return result, nil
	case api.OpTrace:
		var params api.TraceRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		result, err := store.Trace(ctx, params)
		if err != nil {
			return nil, storeError("trace", err)
		}
		return result, nil
	case api.OpSnippet:
		var params api.SnippetRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		result, err := store.Snippet(ctx, params)
		if err != nil {
			return nil, storeError("snippet", err)
		}
		return result, nil
	case api.OpArchitecture:
		var params api.ArchitectureRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		result, err := store.Architecture(ctx, params)
		if err != nil {
			return nil, storeError("architecture", err)
		}
		return result, nil
	case api.OpLayout:
		var params api.LayoutRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		result, err := store.Layout(ctx, params)
		if err != nil {
			return nil, storeError("layout", err)
		}
		return result, nil
	case api.OpImpact:
		var params api.ImpactRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		result, err := store.Impact(ctx, params)
		if err != nil {
			return nil, storeError("impact", err)
		}
		return result, nil
	case api.OpCoverage:
		var params api.CoverageRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		result, err := store.Coverage(ctx, params)
		if err != nil {
			return nil, storeError("coverage", err)
		}
		return result, nil
	case api.OpProjects:
		var params api.StatusRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		store, callErr := openRequestStore(params.RepoRoot)
		if callErr != nil {
			return nil, callErr
		}
		defer store.Close()
		projects, err := store.Projects(ctx)
		if err != nil {
			return nil, storeError("projects", err)
		}
		return api.ProjectsResult{Projects: projects, Page: api.Page{Limit: len(projects), Total: len(projects)}}, nil
	case api.OpProjectDelete:
		var params api.ProjectDeleteRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		paths, err := CachePaths(params.RepoRoot)
		if err != nil {
			return nil, invalidParams(err)
		}
		store, err := OpenWritable(paths.Database)
		if err != nil {
			return nil, storeError("projects_delete", err)
		}
		defer store.Close()
		deleted, err := store.DeleteProject(ctx, params.Project)
		if err != nil {
			return nil, storeError("projects_delete", err)
		}
		return api.ProjectDeleteResult{Project: params.Project, Deleted: deleted}, nil
	case api.OpArtifactExport:
		var params api.ArtifactRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		manifest, err := ExportArtifact(ctx, params.RepoRoot, params.Path)
		if err != nil {
			return nil, storeError("artifact_export", err)
		}
		return artifactResult(params.Path, manifest, true), nil
	case api.OpArtifactImport:
		var params api.ArtifactRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		manifest, live, err := ImportArtifact(ctx, params.RepoRoot, params.Path)
		if err != nil {
			return nil, storeError("artifact_import", err)
		}
		return artifactResult(live, manifest, true), nil
	case api.OpArtifactVerify:
		var params api.ArtifactRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		manifest, err := VerifyArtifact(ctx, params.Path)
		if err != nil {
			return nil, storeError("artifact_verify", err)
		}
		return artifactResult(params.Path, manifest, true), nil
	case api.OpDiagnostics:
		var params api.DiagnosticsRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		result, err := Diagnose(ctx, params.RepoRoot, params.Project)
		if err != nil {
			return nil, storeError("diagnostics", err)
		}
		return result, nil
	case api.OpCodeSearch:
		var params api.CodeSearchRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		root := params.RepoRoot
		if root == "" {
			return nil, invalidParams(fmt.Errorf("repo_root is required"))
		}
		result, err := CodeSearch(ctx, root, params)
		if err != nil {
			return nil, storeError("code_search", err)
		}
		return result, nil
	case api.OpIncremental:
		var params api.IncrementalRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		result, err := IndexIncremental(ctx, params, s.EngineVersion, s.Assets)
		if err != nil {
			return nil, storeError("incremental", err)
		}
		return result, nil
	case api.OpTraceIngest:
		var params api.TraceIngestRequest
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		result, err := TraceIngest(ctx, params.RepoRoot, params)
		if err != nil {
			return nil, storeError("traces_ingest", err)
		}
		return result, nil
	default:
		return nil, &api.Error{
			Code: "capability_not_ready", Message: fmt.Sprintf("operation %q has not passed its readiness gate", req.Operation),
			Suggestion: "use a supported native graph operation",
		}
	}
}

func artifactResult(path string, manifest ArtifactManifest, verified bool) api.ArtifactResult {
	return api.ArtifactResult{Path: path, Project: manifest.Project, Generation: manifest.Generation, DatabaseSHA256: manifest.DatabaseSHA256, DatabaseSize: manifest.DatabaseSize, Verified: verified}
}

func decodeParams(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return nil
}

func openRequestStore(repoRoot string) (*Store, *api.Error) {
	paths, err := CachePaths(repoRoot)
	if err != nil {
		return nil, invalidParams(err)
	}
	store, err := OpenReadOnly(paths.Database)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &api.Error{
				Code: "graph_not_indexed", Message: "no native graph exists for this repository",
				Suggestion: "build the native graph before querying it",
			}
		}
		return nil, storeError("open_graph", err)
	}
	return store, nil
}

func invalidParams(err error) *api.Error {
	return &api.Error{Code: "invalid_params", Message: err.Error()}
}

func storeError(code string, err error) *api.Error {
	return &api.Error{Code: code, Message: err.Error(), Retryable: errors.Is(err, context.DeadlineExceeded)}
}
