// Package pullrequests is the Pull requests view, which takes the pane
// area's place: the open pull requests and issues involving the user,
// across repos or in the focused workspace's GitHub repo, in review
// scopes and by search, and one's checks, comments and diff, with actions
// on it. It reads GitHub with the token gh holds, keeps the lists in
// memory and on disk, and refreshes them within GitHub's rate limits.
package pullrequests

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/diffview"
	"github.com/sviniabanditka/notch-lab/internal/platform/lightbox"
	"github.com/sviniabanditka/notch-lab/internal/platform/markdown"
)

// State is where the view runs: the folder whose GitHub repo "This repo"
// names, and the terminal pane a review goes to ("" for none).
type State struct{ Cwd, Pane string }

// Host is what the view asks of the app.
type Host interface {
	// Paste writes text into a terminal pane, as a review for an Agent.
	Paste(pane, text string) error
	// Update runs fn on the main thread and draws a frame after it.
	Update(fn func())
	// Confirm asks the user to confirm action, and runs then if they do.
	Confirm(message, detail, action string, then func())
	// LeavePullRequests closes the view and shows the workspace again.
	LeavePullRequests()
	// WindowFocused reports whether the app's window has the focus; the
	// view refreshes only then. Main thread only.
	WindowFocused() bool
	// Zoom shows a picture in the window's lightbox. Main thread only.
	Zoom(lightbox.Picture)
}

// View is the view's state across frames. Main thread only; its loads run
// on goroutines and hand their results over through Host.Update.
type View struct {
	host Host
	gh   *client
	open bool
	// stop ends the loads and the refresh of the current opening.
	stop context.CancelFunc
	// focused mirrors Host.WindowFocused for the goroutines.
	focused atomic.Bool

	cwd      string
	repo     repo
	repoDone bool
	// thisRepo limits the lists to repo, while it has one.
	thisRepo bool
	noAuth   bool
	// pending waits for the repo, to run after an opening chose "This
	// repo".
	pending func()

	// inboxes are the lists by filter ("" or owner/name), from memory or
	// the disk snapshot; account is whose they are.
	inboxes  map[string]*inbox
	account  string
	fetching map[string]bool
	listErr  map[string]string
	dir      string
	saveMu   sync.Mutex
	openedAt time.Time
	// firstList logs how long an opening took to show a list.
	firstList bool

	query       string
	found       searchState
	focusSearch bool

	scope    int
	rows     ui.ListState
	selected int
	// focusList and focusDetail move the keyboard there in the next frame.
	focusList, focusDetail bool

	listWidth *listWidthPref
	// narrow is whether the last frame showed the list and the detail by
	// turns, and onList whether it showed the list then; entering waits
	// for an opening's first width to choose.
	narrow, onList, entering bool
	// listFocused is whether the list held the focus in its last frame.
	listFocused bool
	// branchRoom is the header width the branch line was last cut to.
	branchRoom float32

	tab        int
	shown      itemKey
	shownItem  item
	det        *detail
	detErr     string
	details    detailCache
	stopDetail context.CancelFunc
	detailBusy bool
	watchKey   string
	stopChecks context.CancelFunc

	diff        *diffview.View
	diffKey     string
	diffErr     string
	stopDiff    context.CancelFunc
	diffLoading bool
	busy        string

	images *markdown.Images
	prefs  *mergePrefs
	// showBots and showResolved open the folded groups of the overview.
	showBots, showResolved bool
	// checkRuns are the checks of each pull request at its last load, and
	// checksOpen whether its checks show a row each, for the run.
	checkRuns  map[itemKey]checksSummary
	checksOpen map[itemKey]bool
}

// searchState is the last search run with Enter: text as typed, and the
// rows GitHub found for it.
type searchState struct {
	text    string
	rows    []item
	err     string
	loading bool
}

// New makes the view; configDir is where it remembers the merge method
// last used per repo and the list's width, and keeps the lists' snapshot,
// "" for nowhere.
func New(host Host, configDir string) *View {
	v := &View{host: host, gh: newClient("https://api.github.com"), selected: -1, prefs: loadMergePrefs(configDir),
		listWidth: loadListWidth(configDir),
		dir:       configDir, fetching: map[string]bool{}, listErr: map[string]string{}, details: detailCache{},
		checkRuns: map[itemKey]checksSummary{}, checksOpen: map[itemKey]bool{}}
	v.images = markdown.NewImages(host.Update, imageSource, v.gh.imageAuth)
	started := time.Now()
	snap, err := loadSnapshot(configDir)
	if err != nil {
		log.Printf("pullrequests: %s: %v", snapshotPath, err)
	}
	v.inboxes, v.account = snap.Inboxes, snap.Account
	if v.inboxes == nil {
		v.inboxes = map[string]*inbox{}
	}
	log.Printf("pullrequests: snapshot of %d lists read in %v", len(v.inboxes), time.Since(started).Round(time.Microsecond))
	return v
}

// Open shows the view for the focused workspace in st: the lists show at
// once from memory or the disk snapshot and refresh in the background,
// every minute while the view shows and the window has the focus.
func (v *View) Open(st *State) { v.openFor(st) }

// Close hides the view and stops its loads.
func (v *View) Close() { v.close() }

func (v *View) IsOpen() bool { return v.open }

// Row is a pull request or issue of the lists loaded.
type Row struct {
	// Key is owner/name#number.
	Key    string
	Title  string
	Repo   string
	Number int
	Issue  bool
}

// Rows are the rows of every list loaded, in memory or the disk snapshot,
// once each, the newest update first; none once a fetch this run found no
// GitHub sign-in.
func (v *View) Rows() []Row { return v.loadedRows() }

// Show shows the row of Rows with key in the open view: the filter and
// the search clear, "This repo" turns off when its lists do not hold the
// row, and the first scope that holds it is chosen.
func (v *View) Show(key string) { v.reveal(key) }

// Search runs query on GitHub in the open view, as typing it in the
// search field and pressing Enter does.
func (v *View) Search(query string) { v.searchFor(query) }

// Build adds the view in the pane area's place; inset is the room the
// window's own buttons take at the left of its top bar.
func (v *View) Build(c *ui.Context, st *State, inset float32) { v.build(c, st, inset) }
