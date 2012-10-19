package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/sessions"
	"github.com/kless/goconfig"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type Name struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Id        string `json:"id"`
}

type NamesResponse struct {
	Profiles []Name `json:"profiles"`
}

type Genome struct {
	Rs9525638 string `json:"rs9525638"`
	Rs2908004 string `json:"rs2908004"`
	Rs2707466 string `json:"rs2707466"`
	Rs7776725 string `json:"rs7776725"`
	Id        string `json:"id"`
}

func computeBoneStrength(g Genome) (strength int) {
    cortical_thickness_weakness := strings.Count(g.Rs9525638, "T") + strings.Count(g.Rs2707466, "C")
    forearm_bone_mineral_density_weakness := strings.Count(g.Rs2908004, "G") + strings.Count(g.Rs2707466, "C")
    forearm_fracture_risk := strings.Count(g.Rs7776725, "C") + strings.Count(g.Rs2908004, "G") + strings.Count(g.Rs2707466, "C")
    strength = 14 - cortical_thickness_weakness - forearm_bone_mineral_density_weakness - forearm_fracture_risk
    return
}

func buildConfig() (configs map[string]string) {
	configs = make(map[string]string)
	c, _ := config.ReadDefault("config.cfg")
	section := "DEFAULT"
	genotype_scopes := []string{"rs9525638", "rs2908004", "rs2707466", "rs7776725"}
	regular_scopes := []string{"basic", "names"}
	scopes := make([]string, len(genotype_scopes)+len(regular_scopes))
	copy(scopes, regular_scopes)
	copy(scopes[len(regular_scopes):], genotype_scopes)
	configs["genotype_scopes"] = strings.Join(genotype_scopes, "%20")
	configs["scope"] = strings.Join(scopes, " ")
	// Your API credentials and server info
	config_keys := []string{"client_id", "client_secret", "api_uri", "redirect_uri",
		"cookie_secret", "static_path", "session_name", "session_access_token_key", "port"}

	var err error
	for _, value := range config_keys {
		configs[value], err = c.String(section, value)
		if err != nil {
			log.Fatal("You must define %s in your config file", value)
		}
	}
	return
}

func JSONResponse(http_method string, url string, access_token string) (data []byte, status_code int) {
	client := &http.Client{}
	req, err := http.NewRequest(http_method, url, nil)
	req.Header.Add("Authorization", "Bearer "+access_token)
	resp, err := client.Do(req)
	data, err = ioutil.ReadAll(resp.Body)
	status_code = resp.StatusCode
	if err != nil {
		log.Printf(err.Error())
	}
	return
}

func namesByProfile(names NamesResponse) (names_by_profile map[string]string) {
	for _, name := range names.Profiles {
		names_by_profile[name.Id] = fmt.Sprintf("%s %s", name.FirstName, name.LastName)
	}
	return
}

func main() {
	config := buildConfig()
	store := sessions.NewCookieStore([]byte(config["cookie_secret"]))

	templates := template.Must(template.ParseFiles("templates/index.dtml", "templates/result.dtml"))

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(config["static_path"]))))
	http.HandleFunc("/receive_code/", func(w http.ResponseWriter, req *http.Request) {
		receiveCode(w, req, config, store, templates)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		index(w, req, config, store, templates)
	})
	err := http.ListenAndServe(":"+config["port"], nil)
	if err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

func receiveCode(w http.ResponseWriter, req *http.Request, config map[string]string, store *sessions.CookieStore, templates *template.Template) {
	session, _ := store.Get(req, config["session_name"])
	context, _ := url.ParseQuery(req.URL.RawQuery)
	if code, ok := context["code"]; ok {
		auth_code := string(code[0])
		resp, _ := http.PostForm(config["api_uri"]+"/token/",
			url.Values{"client_id": {config["client_id"]},
				"client_secret": {config["client_secret"]},
				"grant_type":    {"authorization_code"},
				"code":          {auth_code},
				"redirect_uri":  {config["redirect_uri"]},
				"scope":         {config["scope"]},
			})
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var t_res TokenResponse
			dec := json.NewDecoder(resp.Body)
			err := dec.Decode(&t_res)
			if err != nil {
				log.Printf(err.Error())
			} else {
				session.Values[config["session_access_token_key"]] = t_res.AccessToken
				session.Save(req, w)
				log.Printf("token is %s", t_res.AccessToken)
				http.Redirect(w, req, "/", 303)
			}
		}
	} else if error_type, ok := context["error"]; ok {
		fmt.Fprintf(w, "%s: %s", string(error_type[0]), string(context["error_description"][0]))
	}
}

func index(w http.ResponseWriter, req *http.Request, config map[string]string, store *sessions.CookieStore, templates *template.Template) {
	session, _ := store.Get(req, config["session_name"])
	token, ok := session.Values[config["session_access_token_key"]]
	if !ok {
		context := map[string]string{
			"path":         req.URL.Path,
			"client_id":    config["client_id"],
			"scope":        config["scope"],
			"redirect_uri": config["redirect_uri"],
		}
		_ = templates.ExecuteTemplate(w, "index.dtml", context)
	} else {
		access_token, _ := token.(string)
		api_uri := config["api_uri"]
		data, status := JSONResponse("GET", api_uri+"/1/names/", access_token)
		if status != 200 {
			// Probably, the auth code expired. Go back home and re-authenticate.
			delete(session.Values, config["session_access_token_key"])
			session.Save(req, w)
			http.Redirect(w, req, "/", 303)
		}
		var names NamesResponse
		var genotypes []Genome
		err := json.Unmarshal(data, &names)
		data, status = JSONResponse("GET", api_uri+"/1/genotype/?locations="+config["genotype_scopes"], access_token)
		err = json.Unmarshal(data, &genotypes)
		if err != nil {
			log.Printf(err.Error())
		}
		/*names_by_profile := namesByProfile(names)*/
		_ = templates.ExecuteTemplate(w, "result.dtml", nil)
	}
}
