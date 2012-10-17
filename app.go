package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
    "github.com/gorilla/sessions"
)

// Your API credentials and server info
var client_id = os.Getenv("CLIENT_ID")
var client_secret = os.Getenv("CLIENT_SECRET")
var api_uri = "https://api.23andme.com"
var redirect_uri = "http://localhost:5000/receive_code/"
var rsid = "i3000001"
var scopes = "basic names " + rsid

// Your session credentials
var cookie_secret = os.Getenv("COOKIE_SECRET")
var static_path = os.Getenv("STATIC_PATH")
var session_name = "api"
var session_access_token_key = "api_access_token"
var store = sessions.NewCookieStore([]byte(cookie_secret))

// Misc
var port = os.Getenv("PORT")
var templates = template.Must(template.ParseFiles("templates/index.dtml", "templates/result.dtml"))

// To parse the JSON responses
type TokenResponse struct {
    AccessToken string `json:"access_token"`
    TokenType string `json:"token_type"`
    ExpiresIn int `json:"expires_in"`
    RefreshToken string `json:"refresh_token"`
    Scope string `json:"scope"`
}

type UserProfileResponse struct {
    Id string `json:"id"`
    Genotyped bool `json:"genotyped"`
}

type UserResponse struct {
    Id string `json:"id"`
    Profiles []UserProfileResponse `json:"profiles"`
}

type ProfileNameResponse struct {
    Id string `json:"id"`
    LastName string `json:"last_name"`
    FirstName string `json:"first_name"`
}

type NamesResponse struct {
    Id string `json:"id"`
    LastName string `json:"last_name"`
    FirstName string `json:"first_name"`
    Profiles []ProfileNameResponse `json:"profiles"`
}

func main() {
	if client_id == "" {
		log.Fatal("CLIENT_ID not defined in your environment")
	}
	if client_secret == "" {
		log.Fatal("CLIENT_SECRET not defined in your environment")
	}
	if port == "" {
		log.Fatal("PORT not defined in your environment")
	}
    if cookie_secret == "" {
        log.Fatal("COOKIE_SECRET not defined in your environment")
    }
    http.Handle("/static/",  http.StripPrefix("/static/", http.FileServer(http.Dir(static_path))))
	http.HandleFunc("/receive_code/", receiveCode)
	http.HandleFunc("/", index)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

func receiveCode(w http.ResponseWriter, req *http.Request) {
    session, _ := store.Get(req, session_name)
	context, _ := url.ParseQuery(req.URL.RawQuery)
	if code, ok := context["code"]; ok {
		auth_code := string(code[0])
		resp, _ := http.PostForm(api_uri+"/token/",
			url.Values{"client_id": {client_id},
				"client_secret": {client_secret},
				"grant_type":    {"authorization_code"},
				"code":          {auth_code},
				"redirect_uri":  {redirect_uri},
				"scope":         {scopes},
			})
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
            var t_res TokenResponse
            dec := json.NewDecoder(resp.Body)
            err := dec.Decode(&t_res)
            if err != nil {
                log.Printf(err.Error())
            } else {
                session.Values[session_access_token_key] = t_res.AccessToken
                session.Save(req, w)
                log.Printf("token is %s", t_res.AccessToken)
                http.Redirect(w, req, "/", 302)
            }
        }
	} else if error_type, ok := context["error"]; ok {
		error_type := string(error_type[0])
		error_description := string(context["error_description"][0])
		fmt.Fprintf(w, "Failed: %s: %s", error_type, error_description)
	}
}

func index(w http.ResponseWriter, req *http.Request) {
    session, _ := store.Get(req, session_name)
    access_token, ok := session.Values[session_access_token_key].(string)
    if !ok {
        context := map[string]string{
            "path":         req.URL.Path,
            "client_id":    client_id,
            "scopes":       scopes,
            "redirect_uri": redirect_uri,
        }
        _ = templates.ExecuteTemplate(w, "index.dtml", context)
    } else {
        client := &http.Client{}

        req, err := http.NewRequest("GET", api_uri+"/1/user/", nil)
        req.Header.Add("Authorization", "Bearer " + access_token)
        resp, err := client.Do(req)
        var u_res UserResponse
        dec := json.NewDecoder(resp.Body)
        err = dec.Decode(&u_res)

        req, err = http.NewRequest("GET", api_uri+"/1/names/", nil)
        req.Header.Add("Authorization", "Bearer " + access_token)
        resp, err = client.Do(req)
        var n_res NamesResponse
        dec = json.NewDecoder(resp.Body)
        err = dec.Decode(&n_res)

        if err != nil {
            log.Printf(err.Error())
        } else {
            log.Printf("user id is %s", u_res.Id)
            log.Printf("names are %s %s", n_res.FirstName, n_res.LastName)
        }
        err = templates.ExecuteTemplate(w, "result.dtml", nil)
    }
}
