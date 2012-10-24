package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/sessions"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
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

const (
	MAXIMUM_BONE_STRENGTH = 14
	API_URI               = "https://api.23andme.com"
)

type BoneStrength struct {
	Score            int
	CorticalStrength int
	ForearmBMD       int
	LowerForearmRisk int
	Description      string
}

type BoneStrengthProfile struct {
	Name         Name
	BoneStrength BoneStrength
}

func descriptionForStrength(strength int) string {
	switch {
	case strength < 5:
		return "weak"
	case strength < 8:
		return "average"
	case strength < 11:
		return "high"
	}
	return "superhuman"
}

func computeBoneStrength(g Genome) (strength BoneStrength) {
	strength.CorticalStrength = 4 - strings.Count(g.Rs9525638, "T") - strings.Count(g.Rs2707466, "C")
	strength.ForearmBMD = 4 - strings.Count(g.Rs2908004, "G") - strings.Count(g.Rs2707466, "C")
	strength.LowerForearmRisk = 6 - strings.Count(g.Rs7776725, "C") - strings.Count(g.Rs2908004, "G") - strings.Count(g.Rs2707466, "C")
	strength.Score = strength.CorticalStrength + strength.ForearmBMD + strength.LowerForearmRisk
	strength.Description = descriptionForStrength(strength.Score)
	return
}

func buildConfig() (configs map[string]string) {
	configs = make(map[string]string)
	genotype_scopes := []string{"rs9525638", "rs2908004", "rs2707466", "rs7776725"}
	regular_scopes := []string{"basic", "names"}
	scopes := make([]string, len(genotype_scopes)+len(regular_scopes))
	copy(scopes, regular_scopes)
	copy(scopes[len(regular_scopes):], genotype_scopes)
	configs["genotype_scopes"] = strings.Join(genotype_scopes, "%20")
	configs["scope"] = strings.Join(scopes, " ")
	// Your API credentials and server info
	config_keys := []string{"CLIENT_ID", "CLIENT_SECRET", "REDIRECT_URI",
		"COOKIE_SECRET", "STATIC_PATH", "SESSION_NAME", "SESSION_ACCESS_TOKEN_KEY", "PORT"}

	var environment_result string
	for _, value := range config_keys {
		environment_result = os.Getenv(value)
		if environment_result == "" {
			log.Fatalf("You must define %s in your environment", value)
		}
		configs[value] = environment_result
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

func namesByProfile(names NamesResponse) (names_by_profile map[string]Name) {
	names_by_profile = make(map[string]Name)
	for _, name := range names.Profiles {
		names_by_profile[name.Id] = name
	}
	return
}

func main() {
	config := buildConfig()
	store := sessions.NewCookieStore([]byte(config["COOKIE_SECRET"]))

	templates := template.Must(template.ParseFiles("templates/_base.dtml", "templates/index.dtml", "templates/result.dtml"))

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(config["STATIC_PATH"]))))
	http.HandleFunc("/receive_code/", func(w http.ResponseWriter, req *http.Request) {
		receiveCode(w, req, config, store, templates)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		index(w, req, config, store, templates)
	})
	err := http.ListenAndServe(":"+config["PORT"], nil)
	if err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}

func receiveCode(w http.ResponseWriter, req *http.Request, config map[string]string, store *sessions.CookieStore, templates *template.Template) {
	session, _ := store.Get(req, config["SESSION_NAME"])
	context, _ := url.ParseQuery(req.URL.RawQuery)
	if code, ok := context["code"]; ok {
		auth_code := string(code[0])
		resp, _ := http.PostForm(API_URI+"/token/",
			url.Values{"client_id": {config["CLIENT_ID"]},
				"client_secret": {config["CLIENT_SECRET"]},
				"grant_type":    {"authorization_code"},
				"code":          {auth_code},
				"redirect_uri":  {config["REDIRECT_URI"]},
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
				session.Values[config["SESSION_ACCESS_TOKEN_KEY"]] = t_res.AccessToken
				session.Save(req, w)
				http.Redirect(w, req, "/", 303)
			}
		}
	} else if error_type, ok := context["error"]; ok {
		fmt.Fprintf(w, "%s: %s", string(error_type[0]), string(context["error_description"][0]))
	}
}

func index(w http.ResponseWriter, req *http.Request, config map[string]string, store *sessions.CookieStore, templates *template.Template) {
	session, _ := store.Get(req, config["SESSION_NAME"])
	token, ok := session.Values[config["SESSION_ACCESS_TOKEN_KEY"]]
	if !ok {
		context := map[string]string{
			"path":         req.URL.Path,
			"client_id":    config["CLIENT_ID"],
			"scope":        config["scope"],
			"redirect_uri": config["REDIRECT_URI"],
		}
		_ = templates.ExecuteTemplate(w, "index", context)
	} else {
		access_token, _ := token.(string)
		data, status := JSONResponse("GET", API_URI+"/1/names/", access_token)
		if status != 200 {
			// Probably, the auth code expired. Go back home and re-authenticate.
			delete(session.Values, config["SESSION_ACCESS_TOKEN_KEY"])
			session.Save(req, w)
			http.Redirect(w, req, "/", 303)
		}
		var names NamesResponse
		var genotypes []Genome
		err := json.Unmarshal(data, &names)
		data, status = JSONResponse("GET", API_URI+"/1/genotype/?locations="+config["genotype_scopes"], access_token)
		err = json.Unmarshal(data, &genotypes)
		if err != nil {
			log.Printf(err.Error())
		}
		names_by_profile := namesByProfile(names)
		var boneStrengthProfiles []BoneStrengthProfile
		for _, genotype := range genotypes {
			boneStrength := computeBoneStrength(genotype)
			boneStrengthProfile := BoneStrengthProfile{
				BoneStrength: boneStrength,
				Name:         names_by_profile[genotype.Id],
			}
			boneStrengthProfiles = append(boneStrengthProfiles, boneStrengthProfile)
		}
		_ = templates.ExecuteTemplate(w, "result", boneStrengthProfiles)
	}
}
