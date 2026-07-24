package main

// Library, Bundle, and Install commands. A Library entry stores an
// install-source descriptor, never a frozen copy, so Install always resolves
// the current version from that source (CONTEXT.md: Library, Install).

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/jnyross/Skill-Manager/internal/engine"
)

func runLibrary(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return fail(stderr, usagef("usage: skillet library list|add|remove"))
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, `usage: skillet library <list|add|remove> [flags]

  list [--json]                          List every Library entry
  add --name NAME <source flags> --yes   Add an entry (see "skillet library add --help")
  remove <id|name> --yes                 Remove an entry (never touches installed files)
`)
		return 0
	case "list":
		return runLibraryList(args[1:], stdout, stderr)
	case "add":
		return runLibraryAdd(args[1:], stdout, stderr)
	case "remove":
		return runLibraryRemove(args[1:], stdout, stderr)
	}
	return fail(stderr, usagef("unknown library subcommand %q: use list, add, or remove", args[0]))
}

func runLibraryList(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet library list", "usage: skillet library list [--json]",
		"Lists the user's own catalog of install-source descriptors, oldest first.")
	asJSON := cmd.flags.Bool("json", false, "emit the Library as JSON")
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 0 {
		return fail(stderr, usagef("library list takes no arguments (got %q)", cmd.flags.Arg(0)))
	}
	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	entries, err := e.ListLibrary()
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if entries == nil {
			entries = []engine.LibraryEntry{}
		}
		if err := writeJSON(stdout, libraryListJSON{SchemaVersion: jsonSchemaVersion, Entries: entries}); err != nil {
			return fail(stderr, err)
		}
		return 0
	}
	if len(entries) == 0 {
		fmt.Fprintln(stdout, "The Library is empty.")
		return 0
	}
	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tNAME\tTOOL\tSOURCE KIND\tSOURCE")
	for _, entry := range entries {
		tool := string(entry.Tool)
		if tool == "" {
			tool = "-"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", entry.ID, entry.Name, tool, entry.Source.Kind, describeLibrarySource(entry.Source))
	}
	_ = table.Flush()
	return 0
}

func describeLibrarySource(source engine.LibrarySource) string {
	switch source.Kind {
	case engine.LibrarySourceLocalPath:
		return source.LocalPath
	case engine.LibrarySourceGit:
		description := source.GitURL
		if source.GitRef != "" {
			description += "@" + source.GitRef
		}
		if source.GitSubPath != "" {
			description += " (" + source.GitSubPath + ")"
		}
		return description
	case engine.LibrarySourceSkillsSh:
		if source.SkillsShSkill != "" {
			return source.SkillsShRepo + "/" + source.SkillsShSkill
		}
		return source.SkillsShRepo
	case engine.LibrarySourceMarketplace:
		return source.PluginName + "@" + source.Marketplace
	}
	return string(source.Kind)
}

func runLibraryAdd(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet library add",
		"usage: skillet library add --name NAME <source flags> [--tool TOOL] [--kind KIND] --yes [--json]",
		`Adds one install-source descriptor to the Library. Give exactly one source:

  --local-path PATH
  --git-url URL [--git-ref REF] [--git-subpath PATH]
  --skills-sh OWNER/REPO [--skills-sh-skill NAME]
  --marketplace NAME --plugin NAME [--marketplace-source SOURCE]

Adding never copies or installs anything; use "skillet install" for that.`)
	name := cmd.flags.String("name", "", "entry name (required)")
	tool := cmd.flags.String("tool", "", "Tool this entry installs for: claude-code or codex")
	kind := cmd.flags.String("kind", "", "skill or prompt (default skill for non-marketplace sources)")
	localPath := cmd.flags.String("local-path", "", "local filesystem path source")
	gitURL := cmd.flags.String("git-url", "", "git URL source")
	gitRef := cmd.flags.String("git-ref", "", "git ref for --git-url")
	gitSubPath := cmd.flags.String("git-subpath", "", "sub-path inside the git repository")
	skillsSh := cmd.flags.String("skills-sh", "", "skills.sh owner/repo source")
	skillsShSkill := cmd.flags.String("skills-sh-skill", "", "single skill inside the skills.sh source (default: all)")
	marketplace := cmd.flags.String("marketplace", "", "marketplace name for a plugin source")
	pluginName := cmd.flags.String("plugin", "", "plugin name for a marketplace source")
	marketplaceSource := cmd.flags.String("marketplace-source", "", "where the marketplace itself comes from")
	asJSON := cmd.flags.Bool("json", false, "emit the created entry as JSON")
	confirmed := cmd.yesFlag()
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 0 {
		return fail(stderr, usagef("library add takes no positional arguments (got %q)", cmd.flags.Arg(0)))
	}
	if strings.TrimSpace(*name) == "" {
		return fail(stderr, usagef("library add requires --name"))
	}
	if err := requireYes(*confirmed, "library add"); err != nil {
		return fail(stderr, err)
	}

	var source engine.LibrarySource
	var chosen []string
	if *localPath != "" {
		chosen = append(chosen, "--local-path")
		source = engine.LibrarySource{Kind: engine.LibrarySourceLocalPath, LocalPath: *localPath}
	}
	if *gitURL != "" {
		chosen = append(chosen, "--git-url")
		source = engine.LibrarySource{Kind: engine.LibrarySourceGit, GitURL: *gitURL, GitRef: *gitRef, GitSubPath: *gitSubPath}
	}
	if *skillsSh != "" {
		chosen = append(chosen, "--skills-sh")
		source = engine.LibrarySource{Kind: engine.LibrarySourceSkillsSh, SkillsShRepo: *skillsSh, SkillsShSkill: *skillsShSkill}
	}
	if *marketplace != "" || *pluginName != "" {
		chosen = append(chosen, "--marketplace/--plugin")
		source = engine.LibrarySource{Kind: engine.LibrarySourceMarketplace, Marketplace: *marketplace, PluginName: *pluginName, MarketplaceSource: *marketplaceSource}
	}
	switch len(chosen) {
	case 1:
	case 0:
		return fail(stderr, usagef("library add requires one source: --local-path, --git-url, --skills-sh, or --marketplace with --plugin"))
	default:
		return fail(stderr, usagef("library add takes exactly one source, got %s", strings.Join(chosen, " and ")))
	}

	entry := engine.LibraryEntry{Name: strings.TrimSpace(*name), Source: source}
	if *kind != "" {
		switch strings.ToLower(*kind) {
		case string(engine.KindSkill):
			entry.Kind = engine.KindSkill
		case string(engine.KindPrompt):
			entry.Kind = engine.KindPrompt
		default:
			return fail(stderr, usagef("unknown --kind %q: use skill or prompt", *kind))
		}
	}
	if *tool != "" {
		parsed, err := parseToolToken(*tool)
		if err != nil {
			return fail(stderr, err)
		}
		entry.Tool = parsed
	} else if source.Kind != engine.LibrarySourceMarketplace {
		entry.Tool = engine.ToolClaudeCode
	}

	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	created, err := e.AddLibraryEntry(entry)
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if err := writeJSON(stdout, libraryEntryJSON{SchemaVersion: jsonSchemaVersion, Entry: created}); err != nil {
			return fail(stderr, err)
		}
		return 0
	}
	fmt.Fprintf(stdout, "Added Library entry %q (id %s, %s source %s)\n",
		created.Name, created.ID, created.Source.Kind, describeLibrarySource(created.Source))
	return 0
}

func runLibraryRemove(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet library remove", "usage: skillet library remove <id|name> --yes",
		"Removes a Library entry. The catalog record goes; nothing installed on disk is\ntouched. An entry still referenced by a Bundle is refused.")
	confirmed := cmd.yesFlag()
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 1 {
		return fail(stderr, usagef("library remove takes exactly one Library entry id or name"))
	}
	if err := requireYes(*confirmed, "library remove"); err != nil {
		return fail(stderr, err)
	}
	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	entry, err := resolveLibraryEntry(e, cmd.flags.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	if err := e.RemoveLibraryEntry(entry.ID); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "Removed Library entry %q (id %s); nothing installed was changed\n", entry.Name, entry.ID)
	return 0
}

func resolveLibraryEntry(e *engine.Engine, arg string) (engine.LibraryEntry, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return engine.LibraryEntry{}, usagef("a Library entry id or name is required")
	}
	entries, err := e.ListLibrary()
	if err != nil {
		return engine.LibraryEntry{}, err
	}
	for _, entry := range entries {
		if entry.ID == arg {
			return entry, nil
		}
	}
	var matches []engine.LibraryEntry
	for _, entry := range entries {
		if strings.EqualFold(entry.Name, arg) {
			matches = append(matches, entry)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return engine.LibraryEntry{}, fmt.Errorf("no Library entry matches %q — run \"skillet library list\" to see the Library", arg)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%q matches %d Library entries — re-run with one of these ids:", arg, len(matches))
	for _, entry := range matches {
		fmt.Fprintf(&builder, "\n  %s  (%s)", entry.ID, describeLibrarySource(entry.Source))
	}
	return engine.LibraryEntry{}, fmt.Errorf("%s", builder.String())
}

func runBundle(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return fail(stderr, usagef("usage: skillet bundle list|install"))
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, `usage: skillet bundle <list|install> [flags]

  list [--json]                                     List Bundles and their members
  install <id|name> --target T --yes [--json]       Install every member of a Bundle
`)
		return 0
	case "list":
		return runBundleList(args[1:], stdout, stderr)
	case "install":
		return runBundleInstall(args[1:], stdout, stderr)
	}
	return fail(stderr, usagef("unknown bundle subcommand %q: use list or install", args[0]))
}

func runBundleList(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet bundle list", "usage: skillet bundle list [--json]",
		"Lists each Bundle with its members and the Activation each member remembers.")
	asJSON := cmd.flags.Bool("json", false, "emit Bundles as JSON")
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 0 {
		return fail(stderr, usagef("bundle list takes no arguments (got %q)", cmd.flags.Arg(0)))
	}
	e, err := newEngineForCWD()
	if err != nil {
		return fail(stderr, err)
	}
	bundles, err := e.ListBundles()
	if err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if bundles == nil {
			bundles = []engine.Bundle{}
		}
		for i := range bundles {
			if bundles[i].Members == nil {
				bundles[i].Members = []engine.BundleMember{}
			}
		}
		if err := writeJSON(stdout, bundleListJSON{SchemaVersion: jsonSchemaVersion, Bundles: bundles}); err != nil {
			return fail(stderr, err)
		}
		return 0
	}
	if len(bundles) == 0 {
		fmt.Fprintln(stdout, "No Bundles.")
		return 0
	}
	entries, _ := e.ListLibrary()
	names := make(map[string]string, len(entries))
	for _, entry := range entries {
		names[entry.ID] = entry.Name
	}
	for _, bundle := range bundles {
		fmt.Fprintf(stdout, "%s  (id %s, %d members)\n", bundle.Name, bundle.ID, len(bundle.Members))
		for _, member := range bundle.Members {
			name := names[member.LibraryEntryID]
			if name == "" {
				name = "(unknown Library entry)"
			}
			fmt.Fprintf(stdout, "  - %s  [%s]  %s\n", name, member.Activation, member.LibraryEntryID)
		}
	}
	return 0
}

func runBundleInstall(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet bundle install", "usage: skillet bundle install <id|name> --target <personal|PATH> --yes [--json]",
		"Installs every member of a Bundle at the target, applying each member's\nremembered Activation. An existing Skill of the same name at the target is\nreplaced.")
	targetValue := cmd.flags.String("target", "", "personal, or a repository path")
	asJSON := cmd.flags.Bool("json", false, "emit the result as JSON")
	confirmed := cmd.yesFlag()
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 1 {
		return fail(stderr, usagef("bundle install takes exactly one Bundle id or name"))
	}
	if err := requireYes(*confirmed, "bundle install"); err != nil {
		return fail(stderr, err)
	}
	target, dir, err := resolveInstallTarget(*targetValue)
	if err != nil {
		return fail(stderr, err)
	}
	e, err := newEngineAt(dir)
	if err != nil {
		return fail(stderr, err)
	}
	bundle, err := resolveBundle(e, cmd.flags.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	if err := e.InstallBundleContext(context.Background(), bundle.ID, target); err != nil {
		return fail(stderr, err)
	}
	if *asJSON {
		if bundle.Members == nil {
			bundle.Members = []engine.BundleMember{}
		}
		document := bundleInstallJSON{
			SchemaVersion: jsonSchemaVersion,
			Bundle:        bundle,
			Target:        newInstallTargetJSON(target),
			Installed:     len(bundle.Members),
		}
		if err := writeJSON(stdout, document); err != nil {
			return fail(stderr, err)
		}
		return 0
	}
	fmt.Fprintf(stdout, "Installed Bundle %q (id %s, %d Library entries) to %s\n",
		bundle.Name, bundle.ID, len(bundle.Members), describeTarget(target))
	return 0
}

func resolveBundle(e *engine.Engine, arg string) (engine.Bundle, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return engine.Bundle{}, usagef("a Bundle id or name is required")
	}
	bundles, err := e.ListBundles()
	if err != nil {
		return engine.Bundle{}, err
	}
	for _, bundle := range bundles {
		if bundle.ID == arg {
			return bundle, nil
		}
	}
	var matches []engine.Bundle
	for _, bundle := range bundles {
		if strings.EqualFold(bundle.Name, arg) {
			matches = append(matches, bundle)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return engine.Bundle{}, fmt.Errorf("no Bundle matches %q — run \"skillet bundle list\" to see the Bundles", arg)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%q matches %d Bundles — re-run with one of these ids:", arg, len(matches))
	for _, bundle := range matches {
		fmt.Fprintf(&builder, "\n  %s", bundle.ID)
	}
	return engine.Bundle{}, fmt.Errorf("%s", builder.String())
}

func runInstall(args []string, stdout, stderr io.Writer) int {
	cmd := newCommand("skillet install", "usage: skillet install <id|name> --target <personal|PATH> [--activation auto|manual-only] --yes",
		"Installs one Library entry at the target, resolving its install-source\ndescriptor fresh. An existing Skill of the same name at the target is\nreplaced.")
	targetValue := cmd.flags.String("target", "", "personal, or a repository path")
	activationValue := cmd.flags.String("activation", "auto", "activation to apply after installing: auto or manual-only")
	confirmed := cmd.yesFlag()
	if code, done := cmd.parse(args, stdout, stderr); done {
		return code
	}
	if cmd.flags.NArg() != 1 {
		return fail(stderr, usagef("install takes exactly one Library entry id or name"))
	}
	if err := requireYes(*confirmed, "install"); err != nil {
		return fail(stderr, err)
	}
	activation, err := parseActivationToken(*activationValue)
	if err != nil {
		return fail(stderr, err)
	}
	target, dir, err := resolveInstallTarget(*targetValue)
	if err != nil {
		return fail(stderr, err)
	}
	e, err := newEngineAt(dir)
	if err != nil {
		return fail(stderr, err)
	}
	entry, err := resolveLibraryEntry(e, cmd.flags.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	destination, _, destErr := e.InstallDestination(entry, target)
	if err := e.InstallLibraryEntryContext(context.Background(), entry, target, activation); err != nil {
		return fail(stderr, err)
	}
	location := ""
	if destErr == nil && destination != "" {
		location = " at " + destination
	}
	fmt.Fprintf(stdout, "Installed Library entry %q (id %s) to %s%s as %s\n",
		entry.Name, entry.ID, describeTarget(target), location, activation)
	return 0
}

func parseActivationToken(token string) (engine.ActivationState, error) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "", "auto":
		return engine.ActivationAuto, nil
	case "manual-only", "manual":
		return engine.ActivationManualOnly, nil
	}
	return "", usagef("unknown --activation %q: use auto or manual-only", token)
}
