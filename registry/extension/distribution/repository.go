package distribution

import (
	"net/http"

	"github.com/distribution/distribution/v3/registry/api/errcode"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/distribution/distribution/v3/registry/extension"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/gorilla/handlers"
)

func repositoryDispatcher(ctx *extension.Context, r *http.Request) http.Handler {
	repositoryHandler := &repositoryHandler{
		Context: ctx,
	}

	return handlers.MethodHandler{
		"DELETE": http.HandlerFunc(repositoryHandler.DeleteRepository),
	}
}

// repositoryHandler handles requests for repository under a repository name.
type repositoryHandler struct {
	*extension.Context
}

// DeleteRepository deletes a repository
func (rh *repositoryHandler) DeleteRepository(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	repo := rh.RepositoryRemover
	if repo == nil {
		rh.Errors = append(rh.Errors, errcode.ErrorCodeUnsupported.WithDetail(nil))
		return
	}

	if err := repo.Remove(rh.Context, rh.Repository.Named()); err != nil {
		switch err := err.(type) {
		case driver.PathNotFoundError:
			rh.Errors = append(rh.Errors, v2.ErrorCodeManifestUnknown.WithDetail(err))
		default:
			rh.Errors = append(rh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
