package extract

import "regexp"

// Category describes a rule group and the reference file it is rendered to.
type Category struct {
	ID    string
	Title string
	File  string // reference file name in the generated skill
}

// Categories in rendering order. "general" must stay last.
var Categories = []Category{
	{"performance", "Performance", "performance.md"},
	{"database", "Database & ORM", "database.md"},
	{"backend-python", "Backend Python", "backend-python.md"},
	{"frontend-owl", "Frontend (OWL / JavaScript)", "frontend-owl.md"},
	{"pos", "Point of Sale", "pos.md"},
	{"security", "Security & Access Rights", "security.md"},
	{"code-reuse", "Code Reuse", "code-reuse.md"},
	{"testing", "Testing", "testing.md"},
	{"compatibility", "Stable Policy & Backward Compatibility", "compatibility.md"},
	{"views-xml", "Views, XML & Data", "views-xml.md"},
	{"i18n", "Translations & User-Facing Text", "i18n.md"},
	{"style", "Readability & Style", "style.md"},
	{"commits", "Commits & PR Hygiene", "commits.md"},
	{"general", "General", "general.md"},
}

// CategoryByID returns the category or "general".
func CategoryByID(id string) Category {
	for _, c := range Categories {
		if c.ID == id {
			return c
		}
	}
	return Categories[len(Categories)-1]
}

// CategoryOrder returns the rendering position of a category id.
func CategoryOrder(id string) int {
	for i, c := range Categories {
		if c.ID == id {
			return i
		}
	}
	return len(Categories)
}

// Signal is a deterministic detector for one recurring kind of reviewer
// feedback, paired with a rule template. A signal only becomes a rule when
// real comments match it, so templates never introduce unevidenced rules.
type Signal struct {
	ID          string
	Category    string
	Subcategory string
	Title       string // the rule statement
	Guidance    string // how to check / apply it
	Severity    string // high|medium|low|context-dependent
	// Weak signals are generic catch-alls ("unnecessary", "readability"):
	// they only apply when no specific signal matched the comment.
	Weak    bool
	Any     []*regexp.Regexp
	All     []*regexp.Regexp // optional extra conditions (all must match)
	PathAny []*regexp.Regexp // optional: at least one must match the file path
}

func rx(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		out[i] = regexp.MustCompile(p)
	}
	return out
}

var posPath = rx(`(^|/)(point_of_sale|pos_[a-z_]+)/`)

// Signals is the ordered taxonomy. Order matters only for tie-breaking.
var Signals = []Signal{
	// --- Performance -------------------------------------------------------
	{ID: "perf-query-in-loop", Category: "performance", Subcategory: "Database queries", Severity: "high",
		Title:    "Avoid database queries inside per-record loops; batch them on the whole recordset.",
		Guidance: "Look for search/browse/read/write/SQL calls inside `for rec in records` loops or per-record computes. Prefetch, group (read_group/_read_group) or move the query out of the loop.",
		Any: rx(`(query|queries|search|browse|read|sql|request|fetch)\w*.{0,60}(loop|for each|every record|each record|per record|each iteration|each line)`,
			`(loop|for each|every record|each record|per record|each line).{0,60}(query|queries|search|browse|sql|request)`,
			`\bn\s*\+\s*1\b`, `one query per`)},
	{ID: "perf-redundant-work", Category: "performance", Subcategory: "Redundant work", Severity: "medium",
		Title:    "Remove redundant queries, RPCs and recomputations.",
		Guidance: "Check whether the same data is fetched or computed twice, or whether a call can be dropped because its result is already available.",
		Any: rx(`(unnecessary|useless|extra|redundant|additional|double|twice)\s+(db |database |sql )?(query|queries|call|calls|computation|loop|iteration|request|requests|rpc|rpcs|read|search|fetch)`,
			`(already|is already) (loaded|fetched|computed|available|in cache|prefetched)`)},
	{ID: "perf-recordset-ops", Category: "performance", Subcategory: "Recordset operations", Severity: "medium",
		Title:    "Prefer recordset operations (mapped, filtered, grouped reads) over manual per-record processing.",
		Guidance: "Replace manual accumulation loops with `mapped`, `filtered`, `sorted`, `_read_group` or set operations on recordsets where it is clearer and cheaper.",
		Any:      rx(`\bmapped\b`, `\bfiltered\b`, `\bread_group\b|_read_group`, `\bprefetch`, `\bgrouped\b`)},
	{ID: "perf-count-exists", Category: "performance", Subcategory: "Database queries", Severity: "medium",
		Title:    "Use search_count, limit=1 or exists-style checks instead of fetching full recordsets.",
		Guidance: "When only presence or a count is needed, do not load records: use `search_count`, `limit=1`, or `_search` with a bounded query.",
		Any:      rx(`search_count`, `limit\s*=\s*1\b`, `\blimit 1\b`, `len\(\s*self\.env`, `len\(.{0,30}search`)},
	{ID: "perf-rpc", Category: "performance", Subcategory: "Frontend round-trips", Severity: "medium",
		Title:    "Minimise RPCs and server round-trips from the frontend; batch or reuse already-loaded data.",
		Guidance: "Check new `orm.call`, `rpc` or `useService(\"orm\")` usages inside loops, getters or render paths.",
		Any:      rx(`\brpcs?\b`, `round[- ]?trips?`, `orm\.(call|read|search)`, `server calls?`)},

	// --- Database & ORM ----------------------------------------------------
	{ID: "orm-raw-sql", Category: "database", Subcategory: "Raw SQL", Severity: "high",
		Title:    "Prefer the ORM over raw SQL; when SQL is needed, use parameters and keep the ORM cache consistent.",
		Guidance: "For `cr.execute`, use the SQL wrapper/parameters, flush the relevant fields before reading, and invalidate caches after writing.",
		Any:      rx(`\braw sql\b`, `\bplain sql\b`, `cr\.execute`, `env\.cr\b`, `\bsql query\b`, `\bin sql\b`)},
	{ID: "orm-flush-invalidate", Category: "database", Subcategory: "ORM cache", Severity: "high",
		Title:    "Flush pending writes before SQL reads and invalidate the cache after SQL writes.",
		Guidance: "Check every raw query: is `flush_model`/`flush_recordset` called before reading, and `invalidate_model`/`invalidate_recordset` after writing?",
		Any:      rx(`\bflush`, `invalidate_(cache|model|recordset|all)`, `\binvalidat\w+ (the )?cache`)},
	{ID: "orm-multi-records", Category: "database", Subcategory: "Recordsets", Severity: "high",
		Title:    "Methods must handle recordsets of any size: loop over self or call ensure_one() explicitly.",
		Guidance: "Check that methods, computes and overrides behave correctly with 0, 1 and many records.",
		Any:      rx(`ensure_one`, `\bsingleton\b`, `expected singleton`, `multi(ple)?[- ]records?`, `several records`, `recordset of (more|several|multiple)`, `self (can|could|may) (be|contain) (several|multiple|many)`)},
	{ID: "orm-compute-depends", Category: "database", Subcategory: "Computed fields", Severity: "medium",
		Title:    "Declare complete @api.depends for computed fields and justify store=True.",
		Guidance: "Verify every field read in the compute is in `@api.depends`, and that stored computes are really needed (search/group/performance).",
		Any:      rx(`@?api\.depends`, `\bdepends on\b.{0,30}(field|compute)`, `computed? fields?`, `store\s*=\s*true`, `\bstored (field|compute)`, `\binverse\b`)},
	{ID: "orm-onchange", Category: "database", Subcategory: "Computed fields", Severity: "medium",
		Title:    "Prefer computed fields (readonly=False) over new @api.onchange logic.",
		Guidance: "New business logic in onchange only runs in the UI; computed fields also apply to imports, RPC and code paths.",
		Any:      rx(`\bonchange\b`, `api\.onchange`)},
	{ID: "orm-crud-override", Category: "database", Subcategory: "Overrides", Severity: "high",
		Title:    "Overrides of create/write/unlink and core methods must call super() and support batch semantics.",
		Guidance: "Use `@api.model_create_multi` with `vals_list`, keep the original return value, and avoid per-record writes inside write().",
		Any: rx(`(override|overrid\w+|overload\w*|inherit\w*)\s.{0,30}\b(create|write|unlink|copy)\b`, `\b(create|write|unlink)\b\s+override`,
			`model_create_multi`, `vals_list`, `call(ing)? super`, `super\(\)\s*(call|is)`)},
	{ID: "orm-domain", Category: "database", Subcategory: "Domains", Severity: "medium",
		Title:    "Filter in the domain (database) instead of filtering records in Python afterwards.",
		Guidance: "Move conditions from `filtered(lambda …)` after a `search` into the search domain when they are expressible there.",
		All:      rx(`\bdomains?\b`),
		Any:      rx(`filter`, `python`, `instead`, `directly`, `search`)},
	{ID: "db-index", Category: "database", Subcategory: "Indexes", Severity: "medium",
		Title:    "Add an index on fields used in frequent searches, joins or domains.",
		Guidance: "Check new fields used in domains, `search` or `_read_group` on large tables for `index=True` (or a specific index type).",
		Any:      rx(`index\s*=\s*(true|'btree|"btree|'trigram|"trigram)`, `\bindex(ed)?\b.{0,20}(field|column|this)`, `(add|missing|need)s? (an )?index`)},
	{ID: "db-constraints", Category: "database", Subcategory: "Constraints", Severity: "medium",
		Title:    "Enforce invariants with SQL or Python constraints instead of ad hoc checks.",
		Guidance: "Use `models.Constraint`/`_sql_constraints` or `@api.constrains` so the invariant holds for every code path.",
		Any:      rx(`_sql_constraints`, `api\.constrains`, `\bconstraints?\b`, `models\.constraint`)},

	// --- Backend Python ----------------------------------------------------
	{ID: "py-exceptions", Category: "backend-python", Subcategory: "Error handling", Severity: "medium",
		Title:    "Raise UserError/ValidationError for user-facing errors and never swallow exceptions with broad except clauses.",
		Guidance: "Check `except Exception`/bare `except:` blocks and user-facing error messages (translated, actionable).",
		Any:      rx(`usererror`, `validationerror`, `bare except`, `except\s*:`, `except exception`, `swallow\w*`, `try\s*/\s*except`, `catch(ing)? (all|every|the) exception`)},
	{ID: "py-context", Category: "backend-python", Subcategory: "Context", Severity: "medium",
		Title:    "Pass behaviour flags through the context deliberately; avoid leaking context keys to unrelated code.",
		Guidance: "Check new `with_context` keys: are they namespaced, documented, and removed before calling unrelated flows?",
		Any:      rx(`with_context`, `context key`, `env\.context`, `self\._context`, `\bin (the )?context\b`)},
	{ID: "py-logging", Category: "backend-python", Subcategory: "Logging", Severity: "low",
		Title:    "Use _logger with lazy %s arguments; never leave print() or debug statements.",
		Guidance: "Search the diff for `print(`, `console.log`, `debugger`, and f-strings passed to `_logger`.",
		Any:      rx(`_logger`, `\bprint\(`, `\bprint statement`, `console\.log`, `\bdebugger\b`, `debug (code|statement|leftover)`)},
	{ID: "py-hardcoded", Category: "backend-python", Subcategory: "Hardcoded values", Severity: "medium",
		Title:    "Avoid hardcoded ids, names and magic values; use env.ref, fields or named constants.",
		Guidance: "Check for literal database ids, hardcoded company/currency/user values and unexplained magic numbers.",
		Any:      rx(`hard[- ]?coded`, `magic (number|string|value)`, `\benv\.ref\b`, `hardcod\w+`)},

	// --- Frontend ----------------------------------------------------------
	{ID: "js-owl-components", Category: "frontend-owl", Subcategory: "OWL components", Severity: "medium",
		Title:    "Follow OWL component conventions: declare props, use hooks correctly and clean up side effects.",
		Guidance: "Check `static props`, `useState`, `onWillStart`, `onMounted`/`onWillUnmount` pairs and that state is not mutated outside reactivity.",
		Any:      rx(`\busestate\b`, `\bonwillstart\b`, `\bonmounted\b`, `\bonwillunmount\b`, `\bonwillupdateprops\b`, `static props`, `\bprops\b.{0,20}(validation|declar|definition)`, `\bowl\b`, `\buseenv\b|\busesubenv\b`)},
	{ID: "js-reactivity-render", Category: "frontend-owl", Subcategory: "Rendering", Severity: "medium",
		Title:    "Avoid unnecessary re-renders and heavy work in getters or templates.",
		Guidance: "Check reactive objects, getters called in templates and loops for expensive computations repeated on every render.",
		Any:      rx(`re-?render`, `\breactive\b`, `\brender(ing)? (again|each|every)`, `\bgetters?\b.{0,40}(each|every|expensive|heavy|render)`)},
	{ID: "js-patch-extend", Category: "frontend-owl", Subcategory: "Extension", Severity: "medium",
		Title:    "Extend frontend behaviour with patch()/inheritance on the right class, not by copying code or monkey-patching.",
		Guidance: "Check that overrides call `super`, target the narrowest class/method, and do not duplicate the original implementation.",
		Any:      rx(`\bpatch\(`, `\bpatch(ing)? (the|this)\b`, `monkey[- ]?patch`)},
	{ID: "js-services", Category: "frontend-owl", Subcategory: "Services", Severity: "medium",
		Title:    "Use services and registries (useService, registry) instead of ad hoc globals or direct imports of state.",
		Guidance: "Look for global singletons, module-level mutable state, or bypassed services.",
		Any:      rx(`useservice`, `\bregistry\b`, `\bservices?\b.{0,30}(instead|should|use)`, `\bglobal (state|variable)`)},
	{ID: "js-dom", Category: "frontend-owl", Subcategory: "DOM", Severity: "medium",
		Title:    "Avoid direct DOM manipulation in OWL code; use templates, refs and state.",
		Guidance: "Check for `querySelector`, `document.`, jQuery or manual class toggling in components.",
		Any:      rx(`queryselector`, `\bdocument\.`, `\bjquery\b`, `\$\(`, `\bdom manipulation`, `direct(ly)? (access|manipulat)\w* (the )?dom`, `\buseref\b`)},
	{ID: "js-async", Category: "frontend-owl", Subcategory: "Async", Severity: "high",
		Title:    "Handle async flows explicitly: await promises and guard against races.",
		Guidance: "Check for un-awaited promises, concurrent clicks, and stale responses (KeepLast, Mutex, disabling buttons).",
		Any:      rx(`race condition`, `\bkeeplast\b`, `\bmutex\b`, `not awaited`, `missing await`, `un-?awaited`, `promise\.all`, `concurren\w+`)},
	{ID: "js-templates", Category: "frontend-owl", Subcategory: "Templates", Severity: "low",
		Title:    "Keep logic out of QWeb/OWL templates and use stable t-key values in t-foreach.",
		Guidance: "Move complex expressions into getters; check `t-key` uniqueness/stability.",
		Any:      rx(`\bt-key\b`, `\bt-foreach\b`, `\bt-if\b.{0,40}(complex|logic|getter)`, `logic in (the )?template`, `\bqweb\b`)},
	{ID: "js-styles", Category: "frontend-owl", Subcategory: "Styling", Severity: "low",
		Title:    "Use existing Bootstrap utility classes and SCSS variables; avoid inline styles and new ad hoc CSS.",
		Guidance: "Check new `style=` attributes, hardcoded colours and CSS that duplicates utility classes.",
		Any:      rx(`\bscss\b`, `inline style`, `style\s*=`, `utility class`, `bootstrap class`, `\bcss\b`)},

	// --- POS ---------------------------------------------------------------
	{ID: "pos-data-loading", Category: "pos", Subcategory: "Data loading", Severity: "high",
		Title:    "Load POS data through the POS data-loading layer and keep frontend models in sync with backend fields.",
		Guidance: "Check `_load_pos_data*`/`_loader_params_*`/`_pos_ui_*` hooks and that only needed fields are loaded.",
		Any:      rx(`_load_pos_data`, `_loader_params`, `_pos_ui_`, `\bloader\b`, `pos\.data`, `data (loading|loaded)`, `load(ed)? in (the )?pos`, `\bfields? (to|are) load`)},
	{ID: "pos-offline", Category: "pos", Subcategory: "Offline & sync", Severity: "high",
		Title:    "Keep POS flows working offline and do not add blocking server calls to order flows.",
		Guidance: "Check new RPCs in payment/validation/order-line paths and that state is synchronised correctly.",
		Any:      rx(`\boffline\b`, `\bsync(hroni[sz]ation|ed|ing)?\b`, `indexeddb`, `local ?storage`, `\bblocking\b`),
		PathAny:  posPath},

	// --- Security ----------------------------------------------------------
	{ID: "sec-sudo", Category: "security", Subcategory: "sudo", Severity: "high",
		Title:    "Justify every sudo(); restrict it to the minimum scope and never apply it to user-controlled input.",
		Guidance: "For each `sudo()` check: why it is needed, which records it touches, and whether results leak to unprivileged users.",
		Any:      rx(`\bsudo\b`)},
	{ID: "sec-access-rights", Category: "security", Subcategory: "Access rights", Severity: "high",
		Title:    "Define access rights and record rules for new models/fields and check access in custom endpoints.",
		Guidance: "Check `ir.model.access.csv`, record rules, `groups=` on sensitive fields and explicit access checks.",
		Any:      rx(`access rights?`, `ir\.model\.access`, `record rules?`, `ir\.rule`, `\bacls?\b`, `\bgroups\s*=`, `check_access`, `\bpermissions?\b`, `multi[- ]?company`)},
	{ID: "sec-injection-xss", Category: "security", Subcategory: "Injection & XSS", Severity: "high",
		Title:    "Never build SQL or HTML from unescaped input: use query parameters, escaping and Markup carefully.",
		Guidance: "Check string-formatted SQL, `t-raw`/`t-out` of user content, `Markup()` on dynamic strings and `innerHTML`.",
		Any:      rx(`sql injection`, `\binjection\b`, `\bxss\b`, `t-raw`, `markup\(`, `innerhtml`, `unescaped`, `\bescap\w+`, `\bsanitiz\w+`)},
	{ID: "sec-controllers", Category: "security", Subcategory: "HTTP routes", Severity: "high",
		Title:    "Secure HTTP routes: correct auth= mode, CSRF handling and validation of all parameters.",
		Guidance: "Check new `@http.route` definitions for `auth`, `csrf`, method restrictions and trust of request data.",
		Any:      rx(`http\.route`, `\bcsrf\b`, `auth\s*=\s*['"]?(public|none|user)`, `\bcontrollers?\b.{0,30}(secur|auth|check)`)},

	// --- Code reuse --------------------------------------------------------
	{ID: "reuse-existing-helper", Category: "code-reuse", Subcategory: "Existing helpers", Severity: "medium",
		Title:    "Search for and reuse existing helpers/methods/components before adding a new implementation.",
		Guidance: "Before writing a utility, grep the module and its dependencies (e.g. `tools`, `float_utils`, existing mixins/components).",
		Any: rx(`(existing|already (a |an )?(have|has|exist\w*)|re-?use)\b.{0,60}\b(helper|method|function|util\w*|implementation|component|tool|mixin)`,
			`\b(helper|method|function|util\w*)s?\b.{0,40}(already exist|exists already|for (that|this))`,
			`duplicat\w+`, `copy[- ]?past\w*`, `\breinvent`)},
	{ID: "reuse-factorize", Category: "code-reuse", Subcategory: "Factorisation", Severity: "low",
		Title:    "Factor logic used in several places into a single function or component.",
		Guidance: "Look for near-identical blocks in the diff (and in the files it touches) that should share one implementation.",
		Any:      rx(`factori[sz]\w*`, `generali[sz]e`, `same (code|logic) (as|in)`, `(extract|move) (this|it) (in|into) a (method|function|helper)`, `in a (separate|dedicated) (method|function)`)},

	// --- Testing -----------------------------------------------------------
	{ID: "test-coverage", Category: "testing", Subcategory: "Coverage", Severity: "medium",
		Title:    "Add or update tests (Python and/or tours) that reproduce the fixed scenario or cover the new feature.",
		Guidance: "A bug fix should include a regression test that fails without the fix whenever feasible.",
		Any:      rx(`(add|missing|need|write|with|without) (a |an |some )?(unit |python |js |hoot |qunit |tour |regression )?tests?\b`, `test case`, `regression test`, `\btours?\b`, `\btests? (for|covering|that)`, `(not|isn't|is not) tested`)},
	{ID: "test-quality", Category: "testing", Subcategory: "Test quality", Severity: "low",
		Title:    "Keep tests deterministic and precise: proper tags, focused assertions, no reliance on demo data or timing.",
		Guidance: "Check `@tagged`, assertions that would also pass with the bug, sleeps/timeouts and dependence on demo records.",
		Any:      rx(`@?tagged`, `post_install`, `\bflaky\b`, `assert(equal|true|in|recordvalues)`, `demo data`, `\bsetupclass\b`)},

	// --- Compatibility -----------------------------------------------------
	{ID: "compat-stable-policy", Category: "compatibility", Subcategory: "Stable policy", Severity: "high",
		Title:    "Respect stable-branch policy: no breaking API, signature, field or data-model changes in stable; target master for refactors.",
		Guidance: "In stable, keep diffs minimal, keep method signatures and returned values, and avoid new stored fields without an upgrade path.",
		Any:      rx(`\bin (a )?stable\b`, `stable (version|branch|policy)`, `\btarget master\b`, `\bmaster only\b`, `\bin master\b`, `\bbreaking (change|api)`, `backward[s]?[- ]compat\w*`, `retro-?compat\w*`, `(change|changing) (the )?signature`, `\bapi change`)},
	{ID: "compat-migration", Category: "compatibility", Subcategory: "Upgrades", Severity: "high",
		Title:    "Provide an upgrade path (migration script or compatible default) when changing stored data or models.",
		Guidance: "Check renamed/removed fields, changed selection values, XML ids and stored computes for upgrade impact.",
		Any:      rx(`\bmigration( script)?s?\b`, `upgrade (script|path)`, `(pre|post|end)-migrat\w+`, `\bupgrade\b.{0,30}(break|issue|impact|database|db)`, `existing (databases|records|data)`)},

	// --- Views / XML -------------------------------------------------------
	{ID: "xml-view-inheritance", Category: "views-xml", Subcategory: "View inheritance", Severity: "medium",
		Title:    "Use precise, robust xpath expressions in view inheritance; avoid fragile positional paths.",
		Guidance: "Prefer `//field[@name='x']` or named elements/`hasclass()` over indexes and long absolute paths.",
		Any:      rx(`\bxpath\b`, `position\s*=`, `inherit_id`, `view inherit\w*`, `hasclass`)},
	{ID: "xml-data-manifest", Category: "views-xml", Subcategory: "Data & manifest", Severity: "medium",
		Title:    "Keep data/demo files and manifest dependencies correct; use noupdate for user-editable data.",
		Guidance: "Check new data files are listed in `__manifest__.py`, dependencies are declared, and records users may edit are `noupdate`.",
		Any:      rx(`noupdate`, `__manifest__`, `\bmanifest\b`, `demo (data|file)`, `data files?`, `\bdepends\b.{0,20}(module|manifest)`)},

	// --- i18n --------------------------------------------------------------
	{ID: "i18n-translatable", Category: "i18n", Subcategory: "Translations", Severity: "medium",
		Title:    "Wrap user-facing strings for translation (_() / _t()) and pass variables as arguments, not via f-strings.",
		Guidance: "Check new error messages, labels and notifications; formatting must happen inside the translation call's arguments.",
		Any:      rx(`\btranslat\w+`, `\b_t\(`, `\b_lt\(`, `\b_\(`, `\bi18n\b`, `\.pot?\b`, `not translat\w+`)},
	{ID: "i18n-wording", Weak: true, Category: "i18n", Subcategory: "Wording", Severity: "low",
		Title:    "Keep user-facing messages clear, actionable and consistent with existing wording.",
		Guidance: "Check spelling, tone and that error messages tell the user what to do.",
		Any:      rx(`\b(typo|wording|phrasing|spelling|rephrase|reword)\b`, `(error|warning) message`, `user[- ]facing`)},

	// --- Style -------------------------------------------------------------
	{ID: "style-naming", Category: "style", Subcategory: "Naming", Severity: "low",
		Title:    "Use clear names that follow Odoo conventions (_compute_x, _onchange_x, x_id/x_ids, model names).",
		Guidance: "Check variable, method and field names for meaning and convention; avoid abbreviations and misleading plurals.",
		Any:      rx(`\bnaming\b`, `\brename\b`, `(better|clearer|more explicit|meaningful|misleading|confusing) name`, `(variable|method|function|field) name`, `\bname (it|this)\b`, `_ids?\b.{0,20}(suffix|convention)`)},
	{ID: "style-simplify", Weak: true, Category: "style", Subcategory: "Simplicity", Severity: "low",
		Title:    "Prefer the simplest correct implementation; remove unnecessary code and indirection.",
		Guidance: "Ask whether each added branch, variable, parameter or abstraction is required for the fix.",
		Any:      rx(`simplif\w+`, `\bsimpler\b`, `can be (written|simplified|reduced)`, `one-?liner`, `\boverkill\b`, `over-?engineer\w*`, `too complex`, `complicated`, `\bnot needed\b`, `\bno need\b`, `\bunneeded\b`, `\bunnecessary\b`)},
	{ID: "style-dead-code", Category: "style", Subcategory: "Dead code", Severity: "low",
		Title:    "Remove dead code, unused variables/imports/parameters and commented-out code.",
		Guidance: "Run linters and search the diff for unused symbols and leftovers.",
		Any:      rx(`dead code`, `\bunused\b`, `not used`, `no longer used`, `\bleftovers?\b`, `commented[- ]out`, `\bnever (used|called)\b`)},
	{ID: "style-readability", Weak: true, Category: "style", Subcategory: "Readability", Severity: "low",
		Title:    "Make non-obvious logic readable: early returns, small functions and a short comment explaining why.",
		Guidance: "Flatten deeply nested conditions and add a comment where the intent is not obvious from the code.",
		Any:      rx(`early return`, `return early`, `guard clause`, `nested (if|condition)`, `(add|missing) (a )?(comment|docstring)`, `explain (why|this)`, `hard to (read|follow|understand)`, `\breadab\w+`)},
	{ID: "style-lint", Category: "style", Subcategory: "Lint & format", Severity: "low",
		Title:    "Follow lint and formatting rules, and do not mix unrelated formatting changes into a fix.",
		Guidance: "Run the project linters (pylint/ruff/eslint) on touched files.",
		Any:      rx(`\bpylint\b`, `\bflake8\b`, `\bruff\b`, `\beslint\b`, `\bprettier\b`, `\blint\w*\b`, `\bpep ?8\b`, `trailing (whitespace|space)`, `indentation`, `line too long`, `import order`, `sort(ed)? imports`)},

	// --- Commits -----------------------------------------------------------
	{ID: "commit-message", Category: "commits", Subcategory: "Commit message", Severity: "medium",
		Title:    "Follow the commit message guidelines: `[TAG] module: summary`, explain the why, and reference the task/ticket.",
		Guidance: "Check the tag ([FIX]/[IMP]/[REF]/[ADD]…), module prefix, a description of the problem and solution, and `task-`/`opw-` references.",
		Any:      rx(`commit (message|msg|title|description)`, `\[(fix|imp|ref|add|rem|mov|rev|merge|i18n|perf|cla)\]`, `\bopw-?\d*`, `\btask-?\d*`, `commit guidelines`)},
	{ID: "commit-structure", Category: "commits", Subcategory: "Commit structure", Severity: "low",
		Title:    "Keep commits and PRs focused: squash fixups and split unrelated changes into separate commits/PRs.",
		Guidance: "Check for fixup commits, unrelated refactors or formatting mixed with the fix.",
		Any:      rx(`\bsquash`, `\bfixup\b`, `split (this|the|into|in)\b.{0,20}commits?`, `separate (commit|pr)`, `\bunrelated\b`, `out of scope`, `not related to (this|the) (pr|commit|fix)`)},
}

// pathCategory derives a category from a file path when text signals are absent.
func pathCategory(path string) (string, string) {
	if path == "" {
		return "", ""
	}
	switch {
	case posPath[0].MatchString(path) && (hasSuffix(path, ".js", ".ts", ".xml", ".scss", ".css") || contains(path, "/static/")):
		return "pos", "Frontend"
	case posPath[0].MatchString(path):
		return "pos", ""
	case contains(path, "/static/src/") || hasSuffix(path, ".js", ".ts", ".scss", ".css"):
		return "frontend-owl", ""
	case contains(path, "/tests/", "/test/") || hasPrefixBase(path, "test_"):
		return "testing", ""
	case contains(path, "/security/"):
		return "security", ""
	case contains(path, "/migrations/", "/upgrades/"):
		return "compatibility", "Upgrades"
	case hasSuffix(path, ".po", ".pot"):
		return "i18n", ""
	case contains(path, "/views/", "/data/", "/report/") || hasSuffix(path, ".xml", ".csv"):
		return "views-xml", ""
	case hasSuffix(path, ".py"):
		return "backend-python", ""
	}
	return "", ""
}
