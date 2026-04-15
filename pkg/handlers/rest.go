package handlers

import "net/http"

type RestHandler interface {
	List(w http.ResponseWriter, r *http.Request)
	Get(w http.ResponseWriter, r *http.Request)
	Create(w http.ResponseWriter, r *http.Request)
	Patch(w http.ResponseWriter, r *http.Request)
	SoftDelete(w http.ResponseWriter, r *http.Request)
}
