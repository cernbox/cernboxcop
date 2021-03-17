package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/spf13/cobra"
	"github.com/studio-b12/gowebdav"
)

var Server string
var Debug bool

func init() {
	rootCmd.AddCommand(impersonateCmd)
	impersonateCmd.PersistentFlags().StringVarP(&Server, "server", "s", "https://cernbox.cern.ch", "Server to do the request")
	impersonateCmd.PersistentFlags().BoolVarP(&Debug, "debug", "d", false, "Show the impersonation token or not")
	impersonateCmd.Flags().StringP("username", "u", "", "Username of user")

	impersonateCmd.AddCommand(getHomeCmd)
	getHomeCmd.Flags().StringP("path", "p", "/", "Optional path to navigate inside")

	impersonateCmd.AddCommand(getProjectsCmd)
	getProjectsCmd.Flags().StringP("project", "n", "", "Project name")
	getProjectsCmd.Flags().StringP("path", "p", "/", "Optional path to navigate inside")

	impersonateCmd.AddCommand(getSharesCmd)
	getSharesCmd.Flags().StringP("shareid", "i", "", "Share id")
	getSharesCmd.Flags().StringP("path", "p", "/", "Optional path to navigate inside")
}

var impersonateCmd = &cobra.Command{
	Use:   "impersonate",
	Short: "Impersonate a user request from the web",
}

var getHomeCmd = &cobra.Command{
	Use:   "files <username> <path>",
	Short: "Retrieve the user files/folders",
	Long:  "This command will list all of the user files in his home directories. Use this to test that accessing his files works correctly.",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) != 1 && len(args) != 2 {
			exit(cmd)
		}
		var username = args[0]

		var path = ""
		if len(args) == 2 {
			path = args[1]
		}

		files, err := getUserFolder(username, filepath.Join("/", path))
		if err != nil {
			er(err)
		}
		listPropfind(files)
	},
}

var getProjectsCmd = &cobra.Command{
	Use:   "projects <username> <project> <path>",
	Short: "Retrieve the projects of a user and list files inside them",
	Long:  "This command will list all of the user projects or the files inside one of them. Use this to test that accessing his projects works correctly.",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) < 1 || len(args) > 3 {
			exit(cmd)
		}
		var username = args[0]

		if len(args) >= 2 {
			var project = args[1]

			var path = ""
			if len(args) == 3 {
				path = args[2]
			}

			files, err := getUserFolder(username, filepath.Join("/__myprojects/", project, path))
			if err != nil {
				er(err)
			}
			listPropfind(files)
		} else {
			// This is not really needed, as we could do a propfind to /, but this way we do the same as the web
			body, err := getOCSAPI(username, "/index.php/apps/files_projectspaces/ajax/personal_list.php?dir=%2F&sort=name&sortdirection=asc")
			if err != nil {
				er(err)
			}

			projects := ProjectsPayload{}
			json.Unmarshal(body, &projects)

			fmt.Println("Name")
			for _, project := range projects.Data.Files {
				fmt.Println(project.Name)
			}
		}
	},
}

type ProjectsPayload struct {
	Data ProjectsData `json:"data"`
}
type ProjectsData struct {
	Files []ProjectStruct `json:"files"`
}
type ProjectStruct struct {
	Name string `json:"name"`
}

var getSharesCmd = &cobra.Command{
	Use:   "shares <username> <shareid> <path>",
	Short: "Retrieve the shares of a user and list files inside them",
	Long:  "This command will list all of the user shares or the files inside one of them. Use this to test that accessing his shares works correctly.",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) < 1 || len(args) > 3 {
			exit(cmd)
		}
		var username = args[0]

		if len(args) >= 2 {
			var share = args[1]

			var path = ""
			if len(args) == 3 {
				path += args[2]
			}
			files, err := getUserFolder(username, filepath.Join("/__myshares/", fmt.Sprintf("(id:%s)", share), path))
			if err != nil {
				er(err)
			}
			listPropfind(files)
		} else {
			body, err := getOCSAPI(username, "/ocs/v1.php/apps/files_sharing/api/v1/shares?format=json&only_shared_by_link=false&only_shared_with_others=false&shared_with_me=true&state=all")
			if err != nil {
				er(err)
			}

			shares := OCSPayload{}
			json.Unmarshal(body, &shares)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.TabIndent)
			fmt.Fprintln(w, "Name\tId\tOwner\tShared with")
			for _, share := range shares.OCS.Data {

				name := share.FileTarget
				name = strings.Replace(name, "/__myshares/", "", 1)
				line := fmt.Sprintf("%s\t%s\t%s\t%s", name, share.ID, share.UIDOwner, share.ShareWith)
				fmt.Fprintln(w, line)
			}
			w.Flush()
		}
	},
}

type OCSPayload struct {
	OCS OCSData `json:"ocs"`
}
type OCSData struct {
	Data []OCSShare `json:"data"`
}
type OCSShare struct {
	ID         string `json:"id"`
	FileTarget string `json:"file_target"`
	ShareWith  string `json:"share_with"`
	UIDOwner   string `json:"uid_owner"`
}

func getUserToken(user string) (string, error) {
	token := jwt.New(jwt.GetSigningMethod("HS256"))
	claims := token.Claims.(jwt.MapClaims)
	claims["account_id"] = user
	claims["groups"] = []string{}
	claims["display_name"] = ""
	claims["exp"] = time.Now().Add(time.Second * time.Duration(3600))
	key := getJwtSigningKey()
	if key == "" {
		return "", errors.New("JWT Signing key not preset")
	}
	tokenStr, err := token.SignedString([]byte(key))
	if err == nil && Debug {
		fmt.Println("Impersonation token: " + tokenStr)
	}

	return tokenStr, err
}

func webdavClient(username string) (*gowebdav.Client, error) {

	token, err := getUserToken(username)
	if err != nil {
		return nil, err
	}
	c := gowebdav.NewClient(Server, "", "")
	c.SetHeader("X-Access-Token", token)
	return c, nil
}

func getOCSAPI(username, path string) ([]byte, error) {

	token, err := getUserToken(username)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s%s", Server, path)

	c := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Access-Token", token)

	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(res.Body)
}

func getUserFolder(username, path string) ([]os.FileInfo, error) {

	c, err := webdavClient(username)
	if err != nil {
		return []os.FileInfo{}, err
	}
	return c.ReadDir(fmt.Sprintf("remote.php/dav/files/%s%s", username, path))
}

func listPropfind(files []os.FileInfo) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.TabIndent)
	fmt.Fprintln(w, "Name\tSize\tModified")
	for _, file := range files {
		line := fmt.Sprintf("%s\t%d\t%s", file.Name(), file.Size(), file.ModTime().Format("2006/01/02 15:04"))
		fmt.Fprintln(w, line)
	}
	w.Flush()
}
