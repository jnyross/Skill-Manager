# Curated cross-harness candidate research

**Issue:** [#45 — Research internet-wide plugin candidates and evidence](https://github.com/jnyross/Skill-Manager/issues/45)

**Parent map:** [#44 — Map curated cross-harness project setup for Skillet](https://github.com/jnyross/Skill-Manager/issues/44)

**Evidence checked:** 2026-07-15

**Status:** research shortlist only; no candidate is approved for Skillet's built-in catalog

## Conclusion

There is enough credible supply to design a useful curated Bundle, but there is not one internet-wide ranking that can safely become the catalog. The strongest general-purpose candidates are:

1. selected skills from [mattpocock/skills](https://github.com/mattpocock/skills), especially the 21 already present in this project;
2. selected, low-side-effect skills from [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills);
3. selected Apache-2.0 skills from [anthropics/skills](https://github.com/anthropics/skills), after Codex verification;
4. either the coherent [obra/superpowers](https://github.com/obra/superpowers) workflow as an optional Bundle or selected skills from [addyosmani/agent-skills](https://github.com/addyosmani/agent-skills), but not both by default because they overlap heavily;
5. [trailofbits/skills](https://github.com/trailofbits/skills) and [dotnet/skills](https://github.com/dotnet/skills) as opt-in domain Bundles, not universal starter content.

Popularity is only a discovery signal. A candidate also needs a traceable upstream, acceptable licensing and attribution, a Skillet-compatible Source, a bounded install footprint, and evidence for both Claude Code and Codex. The current [Agent Skills specification](https://agentskills.io/specification) standardizes the `SKILL.md` package shape, but not every extension used by one harness, and not the distribution or dependency model. The [skills.sh documentation](https://www.skills.sh/docs) says its rankings come from anonymous install telemetry and expressly does not guarantee every listed skill's quality or security. Its counts therefore show CLI install events, not unique active users or fitness for Skillet.

No candidate may enter the approved built-in catalog until John approves the exact Source, included skills/plugins, default Activation, license treatment, and dependency behavior. Repository popularity, official marketplace inclusion, or presence in this project's current `.agents/skills` is not approval.

## Methodology

The survey used primary sources only:

- publisher repositories and their READMEs, manifests, license files, and current GitHub repository API metadata;
- official Claude Code and Codex plugin catalogs and documentation;
- the Agent Skills specification;
- first-party skills.sh install telemetry and CLI documentation.

For each candidate, the review checked:

- **Adoption:** GitHub stars and, where indexed, skills.sh install events. These are comparable signals, not quality scores.
- **Maintenance:** the repository's latest `pushed_at` value as observed through the GitHub API on 2026-07-15. A recent push is evidence of activity, not proof that every contained Skill was reviewed recently.
- **Provenance:** whether the Source belongs to the product/vendor, an identifiable maintainer, or an aggregator.
- **License:** the actual repository or per-Skill license, including attribution/share-alike or source-available constraints.
- **Install:** whether the candidate can be represented by Skillet's existing git, skills.sh, or marketplace Source descriptors without invoking a broad custom installer.
- **Compatibility:** whether upstream provides both Claude Code and Codex instructions/manifests. Publisher documentation is called **documented** compatibility below. **Verified** compatibility remains reserved for the disposable-home live matrix in issue [#48](https://github.com/jnyross/Skill-Manager/issues/48); no candidates were installed during this research.

The comparison intentionally favors exact Skills or small plugins over whole marketplaces. Marketplace admission is useful provenance evidence, but it cannot substitute for reviewing the selected upstream, license, scripts, hooks, external tools, and permissions.

## Evidence comparison

GitHub values are point-in-time observations from each repository's [public repository API](https://docs.github.com/en/rest/repos/repos#get-a-repository). Install counts are from skills.sh pages as retrieved on 2026-07-15.

| Candidate | Adoption and maintenance evidence | License and provenance | Install and compatibility evidence | Assessment |
|---|---|---|---|---|
| [mattpocock/skills](https://github.com/mattpocock/skills) | [171,497 stars; last repository push 2026-07-14](https://api.github.com/repos/mattpocock/skills); [8.8M skills.sh installs across 54 skills](https://www.skills.sh/mattpocock/skills). This project already records 21 of those skills in `skills-lock.json`. | MIT; attributable to Matt Pocock and actively maintained in one source repository. | The [README](https://github.com/mattpocock/skills#quickstart-30-second-setup) uses `npx skills@latest add mattpocock/skills`, documents a native Claude Code plugin, and explicitly says the skills.sh route supports Codex and other Agent-Skills-standard harnesses. | **Shortlist, highest confidence.** Treat the 21 locally used skills as candidate evidence, not implicit approval. Select exact skills; do not automatically import all 54. |
| [obra/superpowers](https://github.com/obra/superpowers) | [255,137 stars; last push 2026-07-14](https://api.github.com/repos/obra/superpowers); [2.3M skills.sh installs across 14 skills](https://www.skills.sh/obra/superpowers). | MIT; identifiable maintainer and dedicated test/eval infrastructure described in the repository. | The [installation guide](https://github.com/obra/superpowers#installation) documents the official Claude plugin marketplace and the official Codex plugin marketplace separately. It also uses hooks/bootstrap behavior and instructs users to install separately in each harness. | **Shortlist as one optional coherent Bundle.** Strong dual-harness evidence, but the skills form an opinionated workflow and should not be mixed piecemeal with an overlapping default workflow without evaluation. |
| [vercel-labs/agent-skills](https://github.com/vercel-labs/agent-skills) | [29,092 stars; last push 2026-07-07](https://api.github.com/repos/vercel-labs/agent-skills); [1.7M skills.sh installs across 13 skills](https://www.skills.sh/vercel-labs/agent-skills), led by `vercel-react-best-practices` (551.5K) and `web-design-guidelines` (463.6K). | The [README declares MIT](https://github.com/vercel-labs/agent-skills#license); official Vercel Labs source. | The repository follows the Agent Skills format and documents `npx skills add vercel-labs/agent-skills`. The [skills CLI compatibility matrix](https://github.com/vercel-labs/skills#supported-agents) includes Claude Code and Codex but warns that hooks and some frontmatter features are harness-specific. | **Shortlist selected Skills.** Prefer low-side-effect guidance such as `web-design-guidelines`, `vercel-react-best-practices`, composition, and writing guidance. Deployment/token Skills need a separate dependency and authority review. |
| [anthropics/skills](https://github.com/anthropics/skills) | [161,323 stars; last push 2026-07-13](https://api.github.com/repos/anthropics/skills); [2.5M skills.sh installs across 18 skills](https://www.skills.sh/anthropics/skills). | Official Anthropic source. Licensing is per Skill: the README says many are Apache-2.0 but `docx`, `pdf`, `pptx`, and `xlsx` are source-available, not open source. For example, [`skill-creator`](https://github.com/anthropics/skills/blob/main/skills/skill-creator/LICENSE.txt), [`frontend-design`](https://github.com/anthropics/skills/blob/main/skills/frontend-design/LICENSE.txt), and [`webapp-testing`](https://github.com/anthropics/skills/blob/main/skills/webapp-testing/LICENSE.txt) carry Apache-2.0 license files. | The [README](https://github.com/anthropics/skills#try-in-claude-code-claudeai-and-the-api) documents a Claude Code marketplace install and describes the repository as Anthropic's implementation of the Agent Skills standard. It does not provide a first-party Codex install recipe. | **Shortlist only individual Apache-2.0 Skills, pending #48.** Do not blanket-approve the repo or the four source-available document Skills. |
| [addyosmani/agent-skills](https://github.com/addyosmani/agent-skills) | [78,471 stars; last push 2026-07-12](https://api.github.com/repos/addyosmani/agent-skills); [205.0K skills.sh installs across 24 skills](https://www.skills.sh/addyosmani). | MIT; attributable to Addy Osmani and a small named maintainer group. | The [README](https://github.com/addyosmani/agent-skills#quick-start) offers selective `npx skills add`, a native Claude marketplace, and a Codex-native plugin manifest/instruction. | **Shortlist selected Skills, not the whole pack.** `source-driven-development`, debugging, interface design, and review are credible candidates, but they overlap with Matt Pocock and Superpowers workflows. Choose one default workflow family after behavior evaluation. |
| [trailofbits/skills](https://github.com/trailofbits/skills) | [6,128 stars; last push 2026-07-07](https://api.github.com/repos/trailofbits/skills); [264.6K skills.sh installs across 79 skills](https://www.skills.sh/trailofbits/skills). | Official Trail of Bits source; CC-BY-SA-4.0, which requires attribution and share-alike treatment rather than the simpler MIT/Apache notice path. | The [README](https://github.com/trailofbits/skills#installation) documents both the Claude Code marketplace and Codex marketplace compatibility. Many plugins call specialized security tools or use multi-agent workflows. | **Shortlist as an opt-in Security Bundle.** Select a small set with explicit prerequisites and complete CC attribution; do not place specialist scanners in the universal starter Bundle. |
| [dotnet/skills](https://github.com/dotnet/skills) | [4,623 stars; last push 2026-07-15](https://api.github.com/repos/dotnet/skills). This is newer and less adopted than the general workflow packs, but activity is current. | MIT; official .NET organization source with contribution and security policies. | The [README](https://github.com/dotnet/skills#installation) documents a Claude Code marketplace and a Codex-native marketplace/individual Skill route, and explicitly ties the skills to the Agent Skills standard. | **Shortlist as an opt-in .NET Bundle.** Strong provenance and dual-harness mechanics, but irrelevant to non-.NET workspaces. |
| [wshobson/agents](https://github.com/wshobson/agents) | [37,927 stars; last push 2026-07-14](https://api.github.com/repos/wshobson/agents). The README reports 94 plugins, 203 agents, 175 skills, and 109 commands. | MIT; identifiable community maintainer, but contains local and external `git-subdir` sources. | The [README](https://github.com/wshobson/agents#multi-harness-support) documents source-to-native generation for Claude, Codex, Cursor, OpenCode, Gemini, and Copilot. Codex installation uses an additional `npx codex-marketplace` tool; some harness artifacts are generated rather than stored as ordinary portable Skills. | **Credible, but hold for adapter research.** The scale and generated multi-surface model are useful evidence for #47, not a safe initial catalog Source without per-plugin review and an install adapter. |
| [garrytan/gstack](https://github.com/garrytan/gstack) | [122,001 stars; last push 2026-07-15](https://api.github.com/repos/garrytan/gstack). | MIT; identifiable maintainer and extensive project documentation. | The [setup instructions](https://github.com/garrytan/gstack#install--30-seconds) require Git and Bun, run a custom setup program, install many skills, edit agent instructions, maintain global/project state, and can perform periodic auto-update checks. The same installer documents Codex and Claude targets. | **Do not include in the first catalog.** Credible and popular, but it is a software factory with broad managed state, binaries, browser components, and mutation semantics—not an ordinary Library entry Skillet can faithfully Install today. |
| [affaan-m/ECC](https://github.com/affaan-m/ECC) | [229,938 stars; last push 2026-07-14](https://api.github.com/repos/affaan-m/ECC). | MIT; identifiable community project. | The repository targets many harnesses, but its own [installation guidance](https://github.com/affaan-m/ECC#pick-one-path-only) warns users not to stack install methods. It includes profiles, hooks, rules, commands, agents, a custom installer, a state store, external integrations, and an alpha control plane in addition to Skills. | **Do not include in the first catalog.** It exceeds Skillet's current Skill/Plugin ownership model and would create unclear rollback, conflict, dependency, and authority boundaries. |
| [googleworkspace/cli](https://github.com/googleworkspace/cli) | [29,707 stars; last push 2026-07-01](https://api.github.com/repos/googleworkspace/cli). | Apache-2.0; official Google Workspace organization source. | The [README](https://github.com/googleworkspace/cli#agent-skills) offers `npx skills add`, but the Skills operate a separate `gws` executable that requires installation, a Google Cloud project, OAuth credentials, and account authorization. | **Defer to a future connector/tool Bundle.** Good provenance, but unsuitable for an agent-ready starter that must not install tools or authenticate services. It also needs explicit external-action safety semantics. |

## Provenance indexes, not blanket candidates

Two official repositories are valuable discovery and provenance indexes but should not be imported wholesale:

- [anthropics/claude-plugins-official](https://github.com/anthropics/claude-plugins-official) [had 32,163 stars and a repository push on 2026-07-15](https://api.github.com/repos/anthropics/claude-plugins-official). Anthropic describes it as an official curated directory, but its README warns that Anthropic does not control third-party plugin contents and directs users to each plugin's own license. Its marketplace manifest pins external sources to commits, which is useful review evidence; the underlying Source remains the licensing and trust boundary.
- [openai/plugins](https://github.com/openai/plugins) [had 4,591 stars and a repository push on 2026-07-14](https://api.github.com/repos/openai/plugins). It is OpenAI's current curated Codex plugin example collection. The former [openai/skills](https://github.com/openai/skills) repository is explicitly deprecated in favor of `openai/plugins`, even though stale ecosystem telemetry still shows [60.3K installs](https://skills.sh/openai/skills). Skillet must not seed new entries from the deprecated repository. Every current OpenAI plugin still needs a Claude-side recipe or an explicit Codex-only classification and its own license/dependency review.

Official marketplace presence means a maintainer accepted a listing; it does not mean John approved it, that the other harness supports it, or that Skillet can own its files safely.

## Proposed shortlist for the approval discussion

### General starter candidates

| Proposed Source | Proposed content boundary | Why it advances |
|---|---|---|
| `mattpocock/skills` | Start from this project's 21 recorded Skills; John selects the exact subset and Activation for the starter Bundle. | Already used in the project, highest observed skills.sh adoption in this survey, MIT, and documented Claude/Codex route. |
| `vercel-labs/agent-skills` | `web-design-guidelines`, `vercel-react-best-practices`, `vercel-composition-patterns`, and/or `writing-guidelines`; exclude deployment/token Skills by default. | Official domain authority, high per-Skill adoption, MIT, ordinary Skills Source. |
| `anthropics/skills` | `skill-creator`, `frontend-design`, and `webapp-testing` only after live Codex checks; exact per-Skill Apache notice retained. | First-party Anthropic examples with clear individual licenses and bounded tasks. |
| `obra/superpowers` **or** selected `addyosmani/agent-skills` | Offer as an optional engineering-workflow Bundle; do not silently combine both with the Matt workflow. | Both have strong adoption, current maintenance, MIT licensing, and documented native routes for Claude and Codex. Their overlap requires a product choice, not an automated popularity decision. |

### Domain-specific candidates

| Proposed Source | Proposed content boundary | Why it is not universal |
|---|---|---|
| `trailofbits/skills` | A small Security Bundle chosen by workflow and executable prerequisites. | Specialist security tasks, CC-BY-SA attribution/share-alike, and tool dependencies. |
| `dotnet/skills` | A .NET Bundle selected only when the workspace is .NET or the user chooses it. | Excellent first-party provenance but domain-specific. |

These are proposals for #46's catalog decision, not entries to create now.

## Rejected or unresolved candidates

- **Reject `openai/skills` as a new Source:** its own README says it is deprecated. Existing stars or install events do not override current upstream direction.
- **Reject whole-marketplace approval:** `anthropics/claude-plugins-official`, `openai/plugins`, and skills.sh are discovery surfaces. Licenses, source owners, dependencies, scripts, and compatibility differ per listing.
- **Reject broad custom installers in the initial catalog:** gstack and ECC are credible projects, but their real installation path owns far more than a Skill directory. Treating them as a normal Library Install would misrepresent conflict handling, rollback, updates, prerequisites, and managed-file ownership.
- **Hold wshobson/agents:** it is a strong reference implementation for Harness Adapters, but its custom generator and extra installer require #47 and #48 decisions before a plugin can graduate.
- **Hold Anthropic's `docx`, `pdf`, `pptx`, and `xlsx` Skills:** upstream explicitly calls them source-available rather than open source. A Source pointer may not copy them into Skillet itself, but redistribution, notice, commercial-use, and derivative behavior must be resolved before promotion.
- **Hold any Skill requiring credentials, paid services, external CLIs, or account mutation:** this includes deployment/token Skills and Google Workspace Skills. Such entries need dependency declarations, preflight, explicit authority boundaries, and configured-unverified behavior that the current catalog model has not yet specified.
- **Do not rank aggregators as favourites:** very large community collections can help discovery, but duplicate provenance, copied Skills, uncertain license inheritance, and mixed maintenance make the original upstream the only acceptable built-in Source.

## Licensing, attribution, and install caveats

1. **Review the exact install unit.** Repository-level license metadata is insufficient when a repository declares per-Skill licenses or references external plugin sources.
2. **Preserve notices.** MIT and Apache-2.0 candidates still need their license/copyright notices preserved. CC-BY-SA candidates need attribution and share-alike handling recorded in the catalog and installed artifact.
3. **Do not call source-available content open source.** Anthropic expressly distinguishes its document Skills from its Apache-2.0 examples.
4. **Treat scripts and hooks as code.** A `SKILL.md` directory may bundle executable scripts; a plugin may add hooks, MCP servers, agents, commands, external binaries, or network access. Review these surfaces and show them before Install.
5. **Documented portability is not behavioral parity.** The Agent Skills base format is portable, but Claude-only frontmatter, hooks, slash commands, subagents, permissions, and tool names may be ignored or behave differently in Codex. #48 must test discovery, invocation, expected files, side effects, and clean removal in both harnesses.
6. **The Source is mutable by design.** ADR 0004 says Install resolves the latest version from the Source instead of a frozen snapshot. Governance should therefore record the upstream URL/path, license, reviewed commit, review date, and approved content boundary, then re-check the diff and license before each built-in catalog release. The reviewed commit is evidence, not an Install pin.
7. **Do not invoke third-party installers invisibly.** `npx skills` can be a source-resolution mechanism Skillet models, but gstack/ECC-style setup programs and extra marketplace generators are materially different install contracts. They need a Harness Adapter or explicit unsupported status.
8. **Keep Activation explicit.** A workflow Skill that automatically triggers can govern large parts of an agent session. The catalog must record and show Auto versus Manual-only per Bundle member instead of assuming upstream defaults.

## Human approval gate

Before #46 can approve an initial catalog, John must decide:

- the exact general-purpose Skills, rather than approving entire repositories;
- whether the starter uses the Matt Pocock workflow family alone, or also offers Superpowers/Addy as separate opt-in Bundles;
- whether CC-BY-SA content is acceptable given attribution and share-alike obligations;
- whether any source-available content may be referenced at all;
- whether entries with external tools, credentials, network services, hooks, or custom installers are excluded from v1 or shown as configured-unverified;
- the default Activation for every approved Bundle member;
- whether an approved Source may advance to latest upstream content automatically at Install time after the built-in catalog ships, or whether catalog-release review metadata must warn when latest differs from the reviewed commit.

Until those answers are recorded, every item in this note remains a research candidate. No repository, marketplace, popularity threshold, or current local installation bypasses this gate.

## Follow-on verification

Issue #48 should exercise each approved candidate in disposable Claude Code and Codex homes and in a disposable Project target. At minimum, verify:

- Source resolution and exact files installed;
- first-session discovery in both harnesses;
- explicit invocation and Auto/Manual-only behavior;
- handling of unsupported frontmatter, scripts, hooks, agents, commands, MCP configuration, and executable prerequisites;
- collision prompts and managed-file ownership;
- repeat Install from latest Source;
- removal/rollback without deleting user-owned files;
- offline and absent-executable behavior reported as configured but unverified.

Only candidates that pass that matrix should be described as verified cross-harness content.
