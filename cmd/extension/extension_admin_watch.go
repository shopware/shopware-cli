package extension

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/shop"

	"github.com/shopware/shopware-cli/internal/asset"

	"github.com/NYTimes/gziphandler"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/spf13/cobra"
	"github.com/vulcand/oxy/v2/forward"

	"github.com/shopware/shopware-cli/extension"
	"github.com/shopware/shopware-cli/internal/esbuild"
	"github.com/shopware/shopware-cli/logging"
)

const schemeHostSeparator = "://"

var (
	hostRegExp              = regexp.MustCompile(`(?m)host:\s'.*,`)
	portRegExp              = regexp.MustCompile(`(?m)port:\s.*,`)
	schemeRegExp            = regexp.MustCompile(`(?m)scheme:\s.*,`)
	schemeAndHttpHostRegExp = regexp.MustCompile(`(?m)schemeAndHttpHost:\s.*,`)
	uriRegExp               = regexp.MustCompile(`(?m)uri:\s.*,`)
	assetPathRegExp         = regexp.MustCompile(`(?m)assetPath:\s.*`)
	assetRegExp             = regexp.MustCompile(`(?m)(src|href|content)="(https?.*\/bundles.*)"`)

	extensionAssetRegExp   = regexp.MustCompile(`(?m)/bundles/([a-z0-9-]+)/static/(.*)$`)
	extensionEsbuildRegExp = regexp.MustCompile(`(?m)/.shopware-cli/([a-z0-9-]+)/(.*)$`)
)

//go:embed static/live-reload.js
var liveReloadJS []byte

var (
	adminWatchListen = ""
	adminWatchURL    = ""
)

var extensionAdminWatchCmd = &cobra.Command{
	Use:   "admin-watch [path] [host]",
	Short: "Extremely fast ESBuild powered Shopware 6 Administration watcher",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		var sources []asset.Source

		for _, extensionPath := range args[:len(args)-1] {
			ext, err := extension.GetExtensionByFolder(extensionPath)
			if err != nil {
				shopCfg, err := shop.ReadConfig(path.Join(extensionPath, shop.DefaultConfigFileName()), true)
				if err != nil {
					return err
				}

				sources = append(sources, extension.FindAssetSourcesOfProject(cmd.Context(), extensionPath, shopCfg)...)
				continue
			}

			sources = append(sources, extension.ConvertExtensionsToSources(cmd.Context(), []extension.Extension{ext})...)
		}

		cfgs := extension.BuildAssetConfigFromExtensions(cmd.Context(), sources, extension.AssetBuildConfig{}).FilterByAdmin()

		if len(cfgs) == 0 {
			return fmt.Errorf("found nothing to compile")
		}

		if _, err := extension.InstallNodeModulesOfConfigs(cmd.Context(), cfgs, false); err != nil {
			return err
		}

		esbuildInstances := make(map[string]adminWatchExtension)

		for name, entry := range cfgs {
			options := esbuild.NewAssetCompileOptionsAdmin(name, entry.BasePath)
			options.ProductionMode = false
			options.DisableSass = entry.DisableSass

			esbuildContext, err := esbuild.Context(cmd.Context(), options)
			if err != nil {
				return err
			}

			if err := esbuildContext.Watch(api.WatchOptions{}); err != nil {
				return err
			}

			watchServer, contextError := esbuildContext.Serve(api.ServeOptions{
				Host: "127.0.0.1",
			})

			if contextError != nil {
				return err
			}

			esbuildInstances[entry.TechnicalName] = adminWatchExtension{
				name:        name,
				assetName:   entry.TechnicalName,
				context:     esbuildContext,
				watchServer: watchServer,
				staticDir:   path.Join(entry.BasePath, "Resources", "app", "static"),
			}
		}

		listenSplit := strings.Split(adminWatchListen, ":")

		if len(listenSplit) != 2 {
			return fmt.Errorf("listen should contain a colon")
		}

		if len(adminWatchURL) == 0 {
			adminWatchURL = "http://localhost:" + listenSplit[1]
		}

		browserUrl, err := url.Parse(adminWatchURL)
		if err != nil {
			return err
		}

		targetShopUrl, err := url.Parse(strings.TrimSuffix(args[len(args)-1], "/"))
		if err != nil {
			return err
		}

		browserPort := browserUrl.Port()

		if len(browserPort) == 0 {
			if browserUrl.Scheme == "https" {
				browserPort = "443"
			} else {
				browserPort = "80"
			}
		}

		fwd := forward.New(true)

		redirect := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			logging.FromContext(cmd.Context()).Debugf("Got request %s %s", req.Method, req.URL.Path)

			// Our custom live reload script
			if req.URL.Path == "/__internal-admin-proxy/live-reload.js" {
				w.Header().Set("content-type", "application/javascript")
				_, _ = w.Write(liveReloadJS)

				return
			}

			assetMatching := extensionAssetRegExp.FindAllString(req.URL.Path, -1)

			if len(assetMatching) > 0 {
				if ext, ok := esbuildInstances[assetMatching[0]]; ok {
					assetPrefix := fmt.Sprintf(targetShopUrl.Path+"/bundles/%s/static/", ext.name)

					http.ServeFile(w, req, path.Join(ext.staticDir, assetPrefix))
					return
				}
			}

			// Modify admin url index page to load anything from our watcher
			if req.URL.Path == targetShopUrl.Path+"/admin" {
				resp, err := http.Get(fmt.Sprintf("%s/admin", targetShopUrl.Scheme+schemeHostSeparator+targetShopUrl.Host))
				if err != nil {
					logging.FromContext(cmd.Context()).Errorf("proxy failed %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					logging.FromContext(cmd.Context()).Errorf("proxy reading failed %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				bodyStr := string(body)

				bodyStr = hostRegExp.ReplaceAllString(bodyStr, "host: '"+browserUrl.Host+"',")
				bodyStr = portRegExp.ReplaceAllString(bodyStr, "port: "+browserPort+",")
				bodyStr = schemeRegExp.ReplaceAllString(bodyStr, "scheme: '"+browserUrl.Scheme+"',")
				bodyStr = schemeAndHttpHostRegExp.ReplaceAllString(bodyStr, "schemeAndHttpHost: '"+browserUrl.Scheme+schemeHostSeparator+browserUrl.Host+"',")
				bodyStr = uriRegExp.ReplaceAllString(bodyStr, "uri: '"+browserUrl.Scheme+schemeHostSeparator+browserUrl.Host+targetShopUrl.Path+"/admin',")
				bodyStr = assetPathRegExp.ReplaceAllString(bodyStr, "assetPath: '"+browserUrl.Scheme+schemeHostSeparator+browserUrl.Host+targetShopUrl.Path+"'")

				bodyStr = assetRegExp.ReplaceAllStringFunc(bodyStr, func(s string) string {
					firstPart := ""

					if strings.HasPrefix(s, "href=\"") {
						firstPart = "href=\""
					} else if strings.HasPrefix(s, "content=\"") {
						firstPart = "content=\""
					} else if strings.HasPrefix(s, "src=\"") {
						firstPart = "src=\""
					}

					org := s
					s = strings.TrimPrefix(s, firstPart)
					s = strings.TrimSuffix(s, "\"")

					parsedUrl, err := url.Parse(s)
					if err != nil {
						logging.FromContext(cmd.Context()).Infof("cannot parse url: %s, err: %s", s, err.Error())
						return org
					}

					if parsedUrl.Host != targetShopUrl.Host {
						return org
					}

					parsedUrl.Host = browserUrl.Host
					parsedUrl.Scheme = browserUrl.Scheme

					return firstPart + parsedUrl.String() + "\""
				})

				w.Header().Set("content-type", "text/html")
				if _, err := w.Write([]byte(bodyStr)); err != nil {
					logging.FromContext(cmd.Context()).Error(err)
				}
				logging.FromContext(cmd.Context()).Debugf("Served modified admin")
				return
			}

			// Inject our custom extension JS
			if req.URL.Path == targetShopUrl.Path+"/api/_info/config" {
				logging.FromContext(cmd.Context()).Debugf("intercept plugins call")

				proxyReq, _ := http.NewRequest("GET", targetShopUrl.Scheme+schemeHostSeparator+targetShopUrl.Host+req.URL.Path, nil)

				proxyReq.Header.Set("Authorization", req.Header.Get("Authorization"))

				resp, err := http.DefaultClient.Do(proxyReq)
				if err != nil {
					logging.FromContext(cmd.Context()).Errorf("proxy failed %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					logging.FromContext(cmd.Context()).Errorf("proxy reading failed %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				var bundleInfo adminBundlesInfo
				if err := json.Unmarshal(body, &bundleInfo); err != nil {
					logging.FromContext(cmd.Context()).Errorf("could not decode bundle info %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if bundleInfo.Bundles == nil {
					logging.FromContext(cmd.Context()).Errorf("cannot inject bundles. got invalid response %s", body)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				for name, bundle := range bundleInfo.Bundles {
					newCss := []string{}

					for _, assetUrl := range bundle.Css {
						parsedUrl, _ := url.Parse(assetUrl)

						if parsedUrl.Host == targetShopUrl.Host {
							parsedUrl.Host = browserUrl.Host
							parsedUrl.Scheme = browserUrl.Scheme
						}

						newCss = append(newCss, parsedUrl.String())
					}

					newJS := []string{}

					for _, assetUrl := range bundle.Js {
						parsedUrl, _ := url.Parse(assetUrl)
						if parsedUrl.Host == targetShopUrl.Host {
							parsedUrl.Host = browserUrl.Host
							parsedUrl.Scheme = browserUrl.Scheme
						}

						newJS = append(newJS, parsedUrl.String())
					}

					bundleInfo.Bundles[name] = adminBundlesInfoAsset{Css: newCss, Js: newJS}
				}

				for _, ext := range esbuildInstances {
					bundleInfo.Bundles[ext.name] = adminBundlesInfoAsset{
						Css:        []string{fmt.Sprintf("%s/.shopware-cli/%s/extension.css", browserUrl.String(), ext.assetName)},
						Js:         []string{fmt.Sprintf("%s/.shopware-cli/%s/extension.js", browserUrl.String(), ext.assetName)},
						LiveReload: true,
						Name:       ext.assetName,
					}
				}

				bundleInfo.Bundles["ShopwareCLI"] = adminBundlesInfoAsset{Css: []string{}, Js: []string{browserUrl.String() + "/__internal-admin-proxy/live-reload.js"}}

				newJson, err := json.Marshal(bundleInfo)
				if err != nil {
					logging.FromContext(cmd.Context()).Errorf("could not encode bundle info %v", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				w.Header().Set("content-type", "application/json")
				if _, err := w.Write(newJson); err != nil {
					logging.FromContext(cmd.Context()).Error(err)
				}

				return
			}

			esbuildMatch := extensionEsbuildRegExp.FindStringSubmatch(req.URL.Path)

			if len(esbuildMatch) > 0 {
				if ext, ok := esbuildInstances[esbuildMatch[1]]; ok {
					req.URL = &url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", ext.watchServer.Host, ext.watchServer.Port), Path: "/" + esbuildMatch[2]}
					req.Host = req.URL.Host
					req.RequestURI = req.URL.Path

					fwd.ServeHTTP(w, req)
					return
				}
			}

			// let us forward this request to another server
			req.URL = targetShopUrl
			fwd.ServeHTTP(w, req)
		})

		wrapper, _ := gziphandler.GzipHandlerWithOpts(gziphandler.ContentTypes([]string{"application/vnd.api+json", "application/json ", "text/html", "text/javascript", "text/css", "image/png"}))

		s := &http.Server{
			Addr:              adminWatchListen,
			Handler:           wrapper(redirect),
			ReadHeaderTimeout: time.Second,
		}
		logging.FromContext(cmd.Context()).Infof("Admin Watcher started at %s%s/admin", browserUrl.String(), targetShopUrl.Path)
		if err := s.ListenAndServe(); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionAdminWatchCmd)
	extensionAdminWatchCmd.PersistentFlags().StringVar(&adminWatchListen, "listen", ":8080", "Listen (default :8080)")
	extensionAdminWatchCmd.PersistentFlags().StringVar(&adminWatchURL, "external-url", "", "External reachable url for admin watcher. Needed for reverse proxy setups")
}

type adminBundlesInfo struct {
	Version         string `json:"version"`
	VersionRevision string `json:"versionRevision"`
	AdminWorker     struct {
		EnableAdminWorker bool     `json:"enableAdminWorker"`
		Transports        []string `json:"transports"`
	} `json:"adminWorker"`
	Bundles  map[string]adminBundlesInfoAsset `json:"bundles"`
	Settings struct {
		EnableUrlFeature  bool `json:"enableUrlFeature"`
		AppUrlReachable   bool `json:"appUrlReachable"`
		AppsRequireAppUrl bool `json:"appsRequireAppUrl"`
	} `json:"settings"`
}

type adminBundlesInfoAsset struct {
	Css        []string `json:"css"`
	Js         []string `json:"js"`
	LiveReload bool     `json:"liveReload"`
	Name       string   `json:"name"`
}

type adminWatchExtension struct {
	name        string
	assetName   string
	context     api.BuildContext
	watchServer api.ServeResult
	staticDir   string
}
