package cmd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectGetOwnerCmd)
	projectCmd.AddCommand(projectUpdateSvcAccount)
	projectCmd.AddCommand(projectDeleteCmd)
	projectCmd.AddCommand(projectAddCmd)
	projectCmd.AddCommand(projectOrphanCmd)

	projectListCmd.Flags().StringP("owner", "o", "", "filter by owner account")
	projectOrphanCmd.Flags().BoolP("quiet", "q", false, "Only show projects name")
	projectListCmd.Flags().BoolP("printpath", "", false, "print EOS path, it may take a while to run")

}

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Project Spaces",
}

var projectAddCmd = &cobra.Command{
	Use:   "add <project-name> <svc-account>",
	Short: "Adds a new project (in db only)",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 2 {
			exit(cmd)
		}

		name := strings.TrimSpace(args[0])
		owner := strings.TrimSpace(args[1])

		if name == "" || owner == "" {
			err := errors.New("project name or owner is empty")
			er(err)
		}

		if err := addProject(name, owner); err != nil {
			er(err)
		}
	},
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete <project-name>",
	Short: "Deletes a project (in db only)",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			exit(cmd)
		}

		projectNameOrPath := strings.TrimSpace(args[0])

		project, err := getProject(projectNameOrPath)
		if err != nil {
			er(err)
		}

		if err := deleteProject(project); err != nil {
			er(err)
		}
	},
}

var projectOrphanCmd = &cobra.Command{
	Use:   "orphan",
	Short: "List only the projects which are in the DB but not in EOS",
	Run: func(cmd *cobra.Command, args []string) {

		quiet, err := cmd.Flags().GetBool("quiet")
		if err != nil {
			er(err)
		}

		cacheInitials := make(map[string]bool)
		cacheProjectsName := make(map[string]bool)

		filterOrphan := func(pSpace *projectSpace) bool {
			splitted := strings.SplitN(pSpace.rel, "/", 2) // splitted = [<initial letter> <project name>]
			initialLetter := splitted[0]
			projectName := splitted[1]

			if !cacheInitials[initialLetter] {
				// not in cache
				// i should retrieve all project starting with the initial letter from EOS
				mgm := fmt.Sprintf("root://eosproject-%s.cern.ch", initialLetter)
				path := fmt.Sprintf("/eos/project/%s", initialLetter)

				files := getFilesInDirEOS(mgm, path)

				// put in cache
				cacheInitials[initialLetter] = true
				for _, file := range files {
					cacheProjectsName[file] = true
				}
			}
			return !cacheProjectsName[projectName]

		}

		orphanSpaces := getProjectsFiltered(filterOrphan)

		if quiet {
			for _, orphan := range orphanSpaces {
				fmt.Println(orphan.name)
			}
		} else {
			printProjectSpaces(orphanSpaces)
		}
	},
}

func getFilesInDirEOS(mgm, pathDir string) []string {
	cmd := exec.Command("eos", mgm, "ls", pathDir)
	out, err := cmd.Output()
	if err != nil {
		er(err)
	}
	return strings.Split(string(out), "\n")
}

var projectUpdateSvcAccount = &cobra.Command{
	Use:   "update-svc-account <project-name> <svc-account>",
	Short: "Update the ownership of a project space (in db only)",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 2 {
			exit(cmd)
		}

		projectNameOrPath := strings.TrimSpace(args[0])
		owner := strings.TrimSpace(args[1])

		project, err := getProject(projectNameOrPath)
		if err != nil {
			er(err)
		}

		if err := updateProjectServiceAccount(project, owner); err != nil {
			er(err)
		}
	},
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all project spaces",
	Run: func(cmd *cobra.Command, args []string) {
		owner, _ := cmd.Flags().GetString("owner")
		projects := getProjectSpaces(owner)

		cols := []string{"Name", "RelativePath", "Owner", "Path"}
		rows := [][]string{}
		printpath, _ := cmd.Flags().GetBool("printpath")
		for i := range projects {
			row := []string{projects[i].name, projects[i].rel, projects[i].owner}
			if printpath {
				row = append(row, projects[i].GetPath())
			}
			rows = append(rows, row)
		}
		pretty(cols, rows)
	},
}

var projectGetOwnerCmd = &cobra.Command{
	Use:   "getowner <project name or path>",
	Short: "Gets the owner of a project space",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			exit(cmd)
		}

		nameOrPath := strings.TrimSpace(args[0])

		owner, err := getProjectOwner(nameOrPath)
		if err != nil {
			er(err)
		}

		fmt.Println(owner)
	},
}

func printProjectSpaces(projects []*projectSpace) {
	cols := []string{"Name", "RelativePath", "Owner"}
	rows := [][]string{}
	for _, project := range projects {
		rows = append(rows, []string{project.name, project.rel, project.owner})
	}
	pretty(cols, rows)
}

var addProject = func(name, owner string) error {
	db := getDB()

	if name == "" {
		panic("adding a new project: project  name is empty")
	}

	relpath := path.Join(string(name[0]), name)

	stmtString := "INSERT INTO cernbox_project_mapping(project_name, eos_relative_path, project_owner) VALUES(?,?,?)"
	stmt, err := db.Prepare(stmtString)
	if err != nil {
		return err
	}

	_, err = stmt.Exec(name, relpath, owner)
	if err != nil {
		return err
	}
	return nil

}

func getProjectsFiltered(filter func(*projectSpace) bool) (projects []*projectSpace) {
	db := getDB()

	query := "SELECT project_name, eos_relative_path, project_owner FROM cernbox_project_mapping"
	rows, err := db.Query(query)
	if err != nil {
		er(err)
	}
	defer rows.Close()

	for rows.Next() {
		pSpace := new(projectSpace)
		err = rows.Scan(&pSpace.name, &pSpace.rel, &pSpace.owner)
		if err != nil {
			er(err)
		}
		if filter(pSpace) {
			projects = append(projects, pSpace)
		}
	}
	err = rows.Err()
	if err != nil {
		er(err)
	}

	return
}

func deleteProject(project *projectSpace) error {
	db := getDB()

	// ensure name is not empty
	if project.name == "" {
		panic(fmt.Sprintf("project name is empty:%+v", project))
	}

	stmtString := "DELETE FROM cernbox_project_mapping WHERE project_name=?"
	stmt, err := db.Prepare(stmtString)
	if err != nil {
		return err
	}

	_, err = stmt.Exec(project.name)
	if err != nil {
		return err
	}
	return nil
}

func updateProjectServiceAccount(project *projectSpace, newOwner string) error {
	db := getDB()

	stmtString := "UPDATE cernbox_project_mapping SET project_owner=? WHERE project_name=?"
	stmt, err := db.Prepare(stmtString)
	if err != nil {
		return err
	}

	_, err = stmt.Exec(newOwner, project.name)
	if err != nil {
		return err
	}
	return nil
}

func getProjectSpaces(ownerFilter string) (projects []*projectSpace) {
	db := getDB()

	query := "SELECT project_name, eos_relative_path, project_owner FROM cernbox_project_mapping"
	rows, err := db.Query(query)
	if err != nil {
		er(err)
	}
	defer rows.Close()

	var name, relpath, owner string
	for rows.Next() {
		err := rows.Scan(&name, &relpath, &owner)
		if err != nil {
			er(err)
		}

		if ownerFilter == "" || ownerFilter == owner {
			proj := &projectSpace{name: name, rel: relpath, owner: owner}
			projects = append(projects, proj)
		}
	}

	err = rows.Err()
	if err != nil {
		er(err)
	}

	return
}

func getProjectOwner(nameOrPath string) (string, error) {
	relpath := getProjectRelPath(nameOrPath)
	projects := getProjectSpaces("")
	for i := range projects {
		if projects[i].rel == relpath {
			return projects[i].owner, nil
		}

		// give it a try without letter prefix, historically
		// there will be projects like "cernbox" or "ski club" under /eos/project
		base := path.Base(nameOrPath)
		if projects[i].rel == base {
			return projects[i].owner, nil
		}
	}

	return "", errors.New("error finding project owner for relative path: " + relpath)
}

func getProject(nameOrPath string) (*projectSpace, error) {
	relpath := getProjectRelPath(nameOrPath)
	projects := getProjectSpaces("")
	for i := range projects {
		if projects[i].rel == relpath {
			return projects[i], nil
		}

		// give it a try without letter prefix, historically
		// there will be projects like "cernbox" or "ski club" under /eos/project
		base := path.Base(nameOrPath)
		if projects[i].rel == base {
			return projects[i], nil
		}
	}
	return nil, errors.New("not found")
}

// name = cernbox
// path = /eos/project/cernbox/
// path = /eos/project/c/cernbox/
// path = c/cernbox/
// returns relative path: c/cernbox
func getProjectRelPath(nameOrPath string) string {
	base := path.Base(nameOrPath)
	return fmt.Sprintf("%s/%s", string(base[0]), base)
}

type projectSpace struct{ name, rel, owner string }

func (p *projectSpace) GetPath() string {

	fullPath := path.Join("/eos/project", p.rel)
	client := getEOSForProject(p.name)
	ctx := context.Background()
	_, err := client.GetFileInfoByPath(ctx, "root", fullPath)
	if err != nil {
		return "NOT_FOUND"
	}
	return fullPath
}
