package kbtest

// OdooFixture loads a small but realistic review history into f.
// Author of every PR is "me". Comment ids are globally unique.
func OdooFixture(f *Fake) {
	rco := User{Login: "rco-odoo", Type: "User"}
	jpp := User{Login: "jpp-odoo", Type: "User"}
	xyz := User{Login: "xyz-odoo", Type: "User"}
	me := User{Login: "me", Type: "User"}
	bot := User{Login: "robodoo", Type: "User"}

	f.Add(&PR{ID: 9101, Number: 101, Title: "[FIX] sale: avoid recomputing taxes", Author: "me", State: "closed", Merged: true,
		CreatedAt: "2026-01-02T10:00:00Z", UpdatedAt: "2026-01-10T10:00:00Z",
		Inline: []Inline{
			{ID: 1001, User: rco, Path: "addons/sale/models/sale_order.py", Line: nil, OrigLine: IntP(120), Position: nil,
				DiffHunk: "@@ -110,6 +110,9 @@\n for line in self.order_line:\n+    line.product_id.read(['name'])",
				Body:     "Don't use this method here because it triggers an unnecessary database query for every record.",
				HTMLURL:  "https://github.com/odoo/odoo/pull/101#discussion_r1001", ReviewID: 5001, CreatedAt: "2026-01-03T09:00:00Z", UpdatedAt: "2026-01-03T09:00:00Z"},
			{ID: 1002, User: me, Path: "addons/sale/models/sale_order.py", InReplyTo: 1001, Body: "Done, moved out of the loop.",
				HTMLURL: "https://github.com/odoo/odoo/pull/101#discussion_r1002", CreatedAt: "2026-01-04T09:00:00Z", UpdatedAt: "2026-01-04T09:00:00Z"},
		},
		Issue: []Comment{
			{ID: 2001, User: rco, Body: "Please target the 17.0 branch, saas versions are frozen for this kind of change.",
				HTMLURL: "https://github.com/odoo/odoo/pull/101#issuecomment-2001", CreatedAt: "2026-01-02T12:00:00Z", UpdatedAt: "2026-01-02T12:00:00Z"},
			{ID: 2002, User: bot, Body: "@robodoo r+", CreatedAt: "2026-01-09T12:00:00Z", UpdatedAt: "2026-01-09T12:00:00Z"},
		},
		Reviews: []Review{
			{ID: 5001, User: rco, State: "CHANGES_REQUESTED", Body: "Please add a test for this fix.",
				HTMLURL: "https://github.com/odoo/odoo/pull/101#pullrequestreview-5001", SubmittedAt: "2026-01-03T09:00:00Z"},
		},
		Threads: []Thread{{RootID: 1001, Resolved: true, Outdated: true}},
	})

	f.Add(&PR{ID: 9102, Number: 102, Title: "[IMP] point_of_sale: faster product screen", Author: "me", State: "closed", Merged: true,
		CreatedAt: "2026-01-20T10:00:00Z", UpdatedAt: "2026-02-03T10:00:00Z",
		Inline: []Inline{
			{ID: 1101, User: jpp, Path: "addons/point_of_sale/static/src/app/screens/product_screen.js", Line: IntP(40),
				Body:    "Please use the existing helper instead of duplicating this implementation.",
				HTMLURL: "https://github.com/odoo/odoo/pull/102#discussion_r1101", CreatedAt: "2026-01-21T09:00:00Z", UpdatedAt: "2026-01-21T09:00:00Z"},
			{ID: 1102, User: rco, Path: "addons/point_of_sale/views/pos_config_views.xml", Line: IntP(12),
				Body:    "nit: maybe rename this variable to something more explicit",
				HTMLURL: "https://github.com/odoo/odoo/pull/102#discussion_r1102", CreatedAt: "2026-01-22T09:00:00Z", UpdatedAt: "2026-01-22T09:00:00Z"},
		},
		Issue: []Comment{
			{ID: 2101, User: rco, Body: "This does a search for each line in the loop, it will be slow with many records.",
				HTMLURL: "https://github.com/odoo/odoo/pull/102#issuecomment-2101", CreatedAt: "2026-01-23T12:00:00Z", UpdatedAt: "2026-01-23T12:00:00Z"},
		},
		Threads: []Thread{{RootID: 1101, Resolved: true, Outdated: false}},
	})

	f.Add(&PR{ID: 9103, Number: 103, Title: "[FIX] account: portal access", Author: "me", State: "closed", Merged: true,
		CreatedAt: "2026-02-10T10:00:00Z", UpdatedAt: "2026-03-01T10:00:00Z",
		Inline: []Inline{
			{ID: 1201, User: jpp, Path: "addons/account/models/account_move.py", Line: IntP(88),
				Body:    "We already have a helper for that in tools, please reuse it rather than duplicating.",
				HTMLURL: "https://github.com/odoo/odoo/pull/103#discussion_r1201", CreatedAt: "2026-02-11T09:00:00Z", UpdatedAt: "2026-02-11T09:00:00Z"},
			{ID: 1202, User: xyz, Path: "addons/account/models/account_move.py", Line: IntP(140),
				Body:    "You should call super() in this write override.",
				HTMLURL: "https://github.com/odoo/odoo/pull/103#discussion_r1202", CreatedAt: "2026-02-11T10:00:00Z", UpdatedAt: "2026-02-11T10:00:00Z"},
			{ID: 1203, User: rco, Path: "addons/account/controllers/portal.py", Line: IntP(30),
				Body:    "Please remove this sudo().",
				HTMLURL: "https://github.com/odoo/odoo/pull/103#discussion_r1203", CreatedAt: "2026-02-12T09:00:00Z", UpdatedAt: "2026-02-12T09:00:00Z"},
			{ID: 1204, User: me, Path: "addons/account/controllers/portal.py", InReplyTo: 1203,
				Body:      "It is needed because the portal user has no access to the move.",
				CreatedAt: "2026-02-12T10:00:00Z", UpdatedAt: "2026-02-12T10:00:00Z"},
			{ID: 1205, User: rco, Path: "addons/account/controllers/portal.py", InReplyTo: 1203,
				Body: "Ok, fair enough.", CreatedAt: "2026-02-12T11:00:00Z", UpdatedAt: "2026-02-12T11:00:00Z"},
		},
		Issue: []Comment{
			{ID: 2201, User: xyz, Body: "LGTM", CreatedAt: "2026-02-28T12:00:00Z", UpdatedAt: "2026-02-28T12:00:00Z"},
		},
		Reviews: []Review{
			{ID: 5201, User: xyz, State: "APPROVED", Body: "", SubmittedAt: "2026-02-28T12:00:00Z"},
		},
	})

	f.Add(&PR{ID: 9104, Number: 104, Title: "[FIX] stock: picking validation", Author: "me", State: "open",
		CreatedAt: "2026-03-05T10:00:00Z", UpdatedAt: "2026-03-15T10:00:00Z",
		Inline: []Inline{
			{ID: 1301, User: User{Login: "abc-odoo", Type: "User"}, Path: "addons/stock/models/stock_picking.py", Line: IntP(200),
				Body:    "Please add a test for this.",
				HTMLURL: "https://github.com/odoo/odoo/pull/104#discussion_r1301", CreatedAt: "2026-03-06T09:00:00Z", UpdatedAt: "2026-03-06T09:00:00Z"},
		},
		Issue: []Comment{
			{ID: 2301, User: jpp, Body: "Please target the 17.0 branch instead, saas versions are frozen.",
				HTMLURL: "https://github.com/odoo/odoo/pull/104#issuecomment-2301", CreatedAt: "2026-03-06T12:00:00Z", UpdatedAt: "2026-03-06T12:00:00Z"},
			{ID: 2302, User: xyz, Body: "Could you add a regression test? Thanks",
				HTMLURL: "https://github.com/odoo/odoo/pull/104#issuecomment-2302", CreatedAt: "2026-03-07T12:00:00Z", UpdatedAt: "2026-03-07T12:00:00Z"},
		},
	})
}
