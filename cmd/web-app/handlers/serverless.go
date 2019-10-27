package handlers

import (
	"context"
	"net/http"
	"net/url"

	"geeks-accelerator/oss/saas-starter-kit/internal/mid"
	"geeks-accelerator/oss/saas-starter-kit/internal/platform/web"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// Serverless provides support for ensuring serverless resources are available for the user. .
type Serverless struct {
	Renderer     web.Renderer
	MasterDB     *sqlx.DB
	MasterDbHost string
	AwsSession   *session.Session
}

// WaitDb validates the the database is resumed and ready to accept requests.
func (h *Serverless) Pending(ctx context.Context, w http.ResponseWriter, r *http.Request, params map[string]string) error {

	var redirectUri string
	if v, ok := ctx.Value(mid.ServerlessKey).(error); ok && v != nil {
		redirectUri = r.RequestURI
	} else {
		redirectUri = r.URL.Query().Get("redirect")
	}

	if redirectUri == "" {
		redirectUri = "/"
	}

	f := func() (bool, error) {
		svc := rds.New(h.AwsSession)

		res, err := svc.DescribeDBClusters(&rds.DescribeDBClustersInput{})
		if err != nil {
			return false, errors.WithMessage(err, "Failed to list AWS db clusters.")
		}

		var targetCluster *rds.DBCluster
		for _, c := range res.DBClusters {
			if c.Endpoint == nil || *c.Endpoint != h.MasterDbHost {
				continue
			}

			targetCluster = c
		}

		if targetCluster == nil {
			return false, errors.New("Failed to find database cluster.")
		} else if targetCluster.ScalingConfigurationInfo == nil || targetCluster.ScalingConfigurationInfo.MinCapacity == nil {
			return false, errors.New("Database cluster has now scaling configuration.")
		}

		if targetCluster.Capacity != nil && *targetCluster.Capacity > 0 {
			return true, nil
		}

		_, err = svc.ModifyCurrentDBClusterCapacity(&rds.ModifyCurrentDBClusterCapacityInput{
			DBClusterIdentifier:  targetCluster.DBClusterIdentifier,
			Capacity:             targetCluster.ScalingConfigurationInfo.MinCapacity,
			SecondsBeforeTimeout: aws.Int64(10),
			TimeoutAction:        aws.String("ForceApplyCapacityChange"),
		})
		if err != nil {
			return false, err
		}

		return false, nil
	}

	end, err := f()
	if err != nil {
		return web.RenderError(ctx, w, r, err, h.Renderer, TmplLayoutBase, TmplContentErrorGeneric, web.MIMETextHTMLCharsetUTF8)
	}

	if web.RequestIsJson(r) {
		data := map[string]interface{}{
			"redirectUri": redirectUri,
			"statusCode":  http.StatusServiceUnavailable,
		}
		if end {
			data["statusCode"] = http.StatusOK
		}
		return web.RespondJson(ctx, w, data, http.StatusOK)
	}

	if end {
		// Redirect the user to their requested page.
		return web.Redirect(ctx, w, r, redirectUri, http.StatusFound)
	}

	data := map[string]interface{}{
		"statusUrl": "/serverless/pending?redirect=" + url.QueryEscape(redirectUri),
	}
	return h.Renderer.Render(ctx, w, r, TmplLayoutBase, "serverless-db.gohtml", web.MIMETextHTMLCharsetUTF8, http.StatusOK, data)
}
