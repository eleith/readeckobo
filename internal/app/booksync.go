package app

import (
	"net/http"
	"net/url"
)

func (a *App) HandleBookSync(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("deviceToken")
	upstream, known := a.bookSyncUpstream(token)
	if !known {
		http.NotFound(w, r)
		return
	}

	target, err := url.Parse(upstream)
	if err != nil {
		a.Logger.Errorf("Invalid configured book sync upstream: %T", err)
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}

	http.StripPrefix("/booksync/"+token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.serveKoboProxy(w, r, target)
	})).ServeHTTP(w, r)
}

func (a *App) bookSyncUpstream(token string) (string, bool) {
	for _, user := range a.Config.Users {
		if user.Token == token {
			if user.BookSyncURL != "" {
				return user.BookSyncURL, true
			}
			return "https://" + storeAPIHost, true
		}
	}
	return "", false
}
