package web

import (
	"fmt"
	"net/http"
	nurl "net/url"
	"os"
	"path"
	"strings"

	"github.com/dnitsch/aws-cli-auth/internal/config"
	"github.com/dnitsch/aws-cli-auth/internal/util"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	ps "github.com/mitchellh/go-ps"
)

type Web struct {
	datadir  *string
	launcher *launcher.Launcher
	browser  *rod.Browser
}

// New returns an initialised instance of Web struct
func New() *Web {
	ddir := path.Join(util.HomeDir(), fmt.Sprintf(".%s-data", config.SELF_NAME))

	l := launcher.New().
		Headless(false).
		Devtools(false).
		Leakless(true)

	url := l.UserDataDir(ddir).MustLaunch()

	browser := rod.New().
		ControlURL(url).
		MustConnect().NoDefaultDevice()

	return &Web{
		datadir:  &ddir,
		launcher: l,
		browser:  browser,
	}

}

// GetSamlLogin performs a saml login
func (web *Web) GetSamlLogin(conf config.SamlConfig) (string, error) {

	// do not clean up userdata

	// datadir := path.Join(util.GetHomeDir(), fmt.Sprintf(".%s-data", config.SELF_NAME))
	util.WriteDataDir(*web.datadir)

	defer web.browser.MustClose()

	page := web.browser.MustPage(conf.ProviderUrl)

	router := web.browser.HijackRequests()
	defer router.MustStop()

	router.MustAdd(conf.AcsUrl, func(ctx *rod.Hijack) {
		body := ctx.Request.Body()
		_ = ctx.LoadResponse(http.DefaultClient, true)
		ctx.Response.SetBody(body)
	})

	go router.Run()

	wait := page.EachEvent(func(e *proto.PageFrameRequestedNavigation) (stop bool) {
		return e.URL == conf.AcsUrl
	})
	wait()

	saml := strings.Split(page.MustElement(`body`).MustText(), "SAMLResponse=")[1]
	saml = strings.Split(saml, "&")[0]
	return nurl.QueryUnescape(saml)

}

func (web *Web) ClearCache() error {
	errs := []error{}

	if err := os.RemoveAll(*web.datadir); err != nil {
		errs = append(errs, err)
	}
	if err := checkRodProcess(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%v", errs[:])
	}
	return nil
}

// checkRodProcess gets a list running process
// kills any hanging rod browser process from any previous improprely closed sessions
func checkRodProcess() error {
	pids := make([]int, 0)
	ps, err := ps.Processes()
	if err != nil {
		return err
	}
	for _, v := range ps {
		if strings.Contains(v.Executable(), "Chromium") {
			pids = append(pids, v.Pid())
		}
	}
	for _, pid := range pids {
		util.Debugf("Process to be killed as part of clean up: %d", pid)
		if proc, _ := os.FindProcess(pid); proc != nil {
			proc.Kill()
		}
	}
	return nil
}
