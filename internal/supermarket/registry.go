package supermarket

import (
	"embed"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed all:data
var dataFS embed.FS

type Author struct {
	Name  string `json:"name" yaml:"name"`
	Email string `json:"email" yaml:"email"`
}

type ConfigVar struct {
	Key          string `json:"key" yaml:"key"`
	Description  string `json:"description" yaml:"description"`
	DefaultValue string `json:"defaultValue,omitempty" yaml:"defaultValue"`
}

type McpEntry struct {
	ID          string      `json:"id"`
	Name        string      `json:"name" yaml:"name"`
	Description string      `json:"description" yaml:"description"`
	Author      Author      `json:"author" yaml:"author"`
	Transport   string      `json:"transport" yaml:"transport"`
	Icon        string      `json:"icon,omitempty" yaml:"icon"`
	Homepage    string      `json:"homepage,omitempty" yaml:"homepage"`
	Tags        []string    `json:"tags,omitempty" yaml:"tags"`
	URL         string      `json:"url,omitempty" yaml:"url"`
	Command     string      `json:"command,omitempty" yaml:"command"`
	Args        []string    `json:"args,omitempty" yaml:"args"`
	Headers     []ConfigVar `json:"headers,omitempty" yaml:"headers"`
	Env         []ConfigVar `json:"env,omitempty" yaml:"env"`
}

type SkillMetadata struct {
	Author   Author   `json:"author" yaml:"author"`
	Tags     []string `json:"tags,omitempty" yaml:"tags"`
	Homepage string   `json:"homepage,omitempty" yaml:"homepage"`
}

type SkillEntry struct {
	ID          string        `json:"id"`
	Name        string        `json:"name" yaml:"name"`
	Description string        `json:"description" yaml:"description"`
	Metadata    SkillMetadata `json:"metadata" yaml:"metadata"`
	Content     string        `json:"content"`
	Files       []string      `json:"files"`
}

type McpListResponse struct {
	Total int        `json:"total"`
	Page  int        `json:"page"`
	Limit int        `json:"limit"`
	Data  []McpEntry `json:"data"`
}

type SkillListResponse struct {
	Total int          `json:"total"`
	Page  int          `json:"page"`
	Limit int          `json:"limit"`
	Data  []SkillEntry `json:"data"`
}

type TagsResponse struct {
	Tags []string `json:"tags"`
}

type Registry struct {
	mcps   []McpEntry
	skills []SkillEntry
}

func NewRegistry() (*Registry, error) {
	r := &Registry{}
	if err := r.loadMcps(); err != nil {
		return nil, err
	}
	if err := r.loadSkills(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Registry) loadMcps() error {
	mcpsDir := "data/mcps"
	entries, err := fs.ReadDir(dataFS, mcpsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		yamlPath := path.Join(mcpsDir, id, "mcp.yaml")
		data, err := fs.ReadFile(dataFS, yamlPath)
		if err != nil {
			continue
		}

		var mcp McpEntry
		if err := yaml.Unmarshal(data, &mcp); err != nil {
			continue
		}
		mcp.ID = id
		r.mcps = append(r.mcps, mcp)
	}
	return nil
}

func (r *Registry) loadSkills() error {
	skillsDir := "data/skills"
	entries, err := fs.ReadDir(dataFS, skillsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		skillMdPath := path.Join(skillsDir, id, "SKILL.md")
		data, err := fs.ReadFile(dataFS, skillMdPath)
		if err != nil {
			continue
		}

		skill := parseSkillMd(id, data)

		// collect file list
		skillDir := path.Join(skillsDir, id)
		dirEntries, _ := fs.ReadDir(dataFS, skillDir)
		for _, f := range dirEntries {
			if !f.IsDir() {
				skill.Files = append(skill.Files, f.Name())
			}
		}

		r.skills = append(r.skills, skill)
	}
	return nil
}

func parseSkillMd(id string, data []byte) SkillEntry {
	content := string(data)
	skill := SkillEntry{ID: id}

	// parse YAML frontmatter
	if strings.HasPrefix(content, "---\n") {
		end := strings.Index(content[4:], "\n---")
		if end >= 0 {
			frontmatter := content[4 : 4+end]
			skill.Content = strings.TrimSpace(content[4+end+4:])

			var fm struct {
				Name        string        `yaml:"name"`
				Description string        `yaml:"description"`
				Metadata    SkillMetadata `yaml:"metadata"`
			}
			if err := yaml.Unmarshal([]byte(frontmatter), &fm); err == nil {
				skill.Name = fm.Name
				skill.Description = fm.Description
				skill.Metadata = fm.Metadata
			}
		}
	}

	if skill.Name == "" {
		skill.Name = id
	}
	return skill
}

func (r *Registry) ListMcps(q, tag, transport string, page, limit int) McpListResponse {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}

	var filtered []McpEntry
	for _, m := range r.mcps {
		if transport != "" && !strings.EqualFold(m.Transport, transport) {
			continue
		}
		if tag != "" && !containsTagCI(m.Tags, tag) {
			continue
		}
		if q != "" && !matchesQuery(q, m.Name, m.Description, m.Tags) {
			continue
		}
		filtered = append(filtered, m)
	}

	total := len(filtered)
	start := (page - 1) * limit
	if start >= total {
		return McpListResponse{Total: total, Page: page, Limit: limit, Data: []McpEntry{}}
	}
	end := start + limit
	if end > total {
		end = total
	}

	return McpListResponse{
		Total: total,
		Page:  page,
		Limit: limit,
		Data:  filtered[start:end],
	}
}

func (r *Registry) GetMcp(id string) (McpEntry, bool) {
	for _, m := range r.mcps {
		if m.ID == id {
			return m, true
		}
	}
	return McpEntry{}, false
}

func (r *Registry) ListSkills(q, tag string, page, limit int) SkillListResponse {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}

	var filtered []SkillEntry
	for _, s := range r.skills {
		if tag != "" && !containsTagCI(s.Metadata.Tags, tag) {
			continue
		}
		if q != "" && !matchesQuery(q, s.Name, s.Description, s.Metadata.Tags) {
			continue
		}
		filtered = append(filtered, s)
	}

	total := len(filtered)
	start := (page - 1) * limit
	if start >= total {
		return SkillListResponse{Total: total, Page: page, Limit: limit, Data: []SkillEntry{}}
	}
	end := start + limit
	if end > total {
		end = total
	}

	return SkillListResponse{
		Total: total,
		Page:  page,
		Limit: limit,
		Data:  filtered[start:end],
	}
}

func (r *Registry) GetSkill(id string) (SkillEntry, bool) {
	for _, s := range r.skills {
		if s.ID == id {
			return s, true
		}
	}
	return SkillEntry{}, false
}

func (r *Registry) GetSkillFiles(id string) (map[string][]byte, error) {
	skillDir := path.Join("data/skills", id)
	entries, err := fs.ReadDir(dataFS, skillDir)
	if err != nil {
		return nil, err
	}

	files := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(dataFS, path.Join(skillDir, entry.Name()))
		if err != nil {
			continue
		}
		files[entry.Name()] = data
	}
	return files, nil
}

func (r *Registry) ListTags() TagsResponse {
	tagSet := make(map[string]struct{})
	for _, m := range r.mcps {
		for _, t := range m.Tags {
			tagSet[strings.ToLower(t)] = struct{}{}
		}
	}
	for _, s := range r.skills {
		for _, t := range s.Metadata.Tags {
			tagSet[strings.ToLower(t)] = struct{}{}
		}
	}

	tags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return TagsResponse{Tags: tags}
}

func containsTagCI(tags []string, target string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, target) {
			return true
		}
	}
	return false
}

func matchesQuery(q, name, desc string, tags []string) bool {
	q = strings.ToLower(q)
	if strings.Contains(strings.ToLower(name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(desc), q) {
		return true
	}
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}
