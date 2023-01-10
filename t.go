package handler

import (
	"fmt"

	dependencyparse "infra/dependency_parse/kitex_gen/bytedance/bits/dependency_parse"
	git_server "infra/dependency_parse/kitex_gen/bytedance/bits/git_server"
	metaService "infra/dependency_parse/kitex_gen/bytedance/bits/meta"
	"infra/dependency_parse/pkg/business"
	constDefine "infra/dependency_parse/pkg/consts"
	"infra/dependency_parse/pkg/service/gitserver"
	"infra/dependency_parse/pkg/service/jenkins"
	"infra/dependency_parse/pkg/service/meta"

	"code.byted.org/bits/common_lib/consts"
	"code.byted.org/bits/common_lib/utils"
	"code.byted.org/gopkg/facility/fjson"
	"code.byted.org/gopkg/logs"
)

func ParseDependency(ctx context.Context, req *dependencyparse.DependencyParseRequest) (*dependencyparse.DependencyParseResponse, error) {

	err := checkCreateDevTaskParam(ctx, req)
	if err != nil {
		logs.CtxDebug(ctx, "error %+v", err)
		return nil, err
	}

	app, err := meta.QueryAppInfo(ctx, req.UniqueId)
	if err != nil {
		logs.CtxDebug(ctx, "error %+v", err)
		return nil, err
	}

	project, err := getProjectInfoByURL(ctx, app.GitUrl)
	if err != nil {
		logs.CtxDebug(ctx, "error %+v", err)
		return nil, err
	}

	ref := req.GetRef()
	if ref == "" {
		ref = project.DefaultBranch
	}

	components := make([]*dependencyparse.ComponentInfo, 0)
	targets := make([]string, 0)

	if app.TechnologyStack == metaService.TechnologyStack_iOS {
		components, targets, err = business.ParseIosDeps(ctx, app, project, ref)
	} else if app.TechnologyStack == metaService.TechnologyStack_Android {
		components, targets, err = business.ParseAndroidRegisteredDeps(ctx, int64(project.Id), ref)
	} else if app.TechnologyStack == metaService.TechnologyStack_Flutter {
		components, targets, err = business.ParseFlutterYamlDeps(ctx, int64(project.Id), ref)
	} else {
		err = fmt.Errorf("unsupported tech stack %v", app.TechnologyStack)
	}

	if err != nil {
		logs.CtxDebug(ctx, "error %+v", err)
		return nil, err
	}

	resp := dependencyparse.DependencyParseResponse{}

	resp.UniqueType = req.UniqueType
	resp.UniqueId = req.UniqueId
	resp.Ref = &ref
	resp.Components = components
	resp.Targets = targets

	return &resp, nil
}

// 检查传入参数的合法性
func checkCreateDevTaskParam(ctx context.Context, req *dependencyparse.DependencyParseRequest) error {

	if req.UniqueId == 0 || req.UniqueType == 0 {

		err := fmt.Errorf("dependency parse unique_type and unique_id is required, but now they are %v %v", req.UniqueType, req.UniqueId)
		logs.CtxDebug(ctx, "error %v", err)
		return err
	}

	return nil
}

func getProjectInfoByURL(ctx context.Context, gitURL string) (*git_server.Project, error) {
	gitAddr := utils.FormatRepoGit(gitURL, consts.HTTPS)
	info, err := gitserver.GetProjectByRepo(ctx, gitAddr)
	if err != nil {
		logs.CtxDebug(ctx, "error %+v", err)
		return nil, err
	}
	return info, nil
}

func getProjectInfoByProjectId(ctx context.Context, projectId int64) (*git_server.Project, error) {
	info, err := gitserver.GetProjectByID(ctx, projectId)
	if err != nil {
		logs.CtxDebug(ctx, "error %+v", err)
		return nil, err
	}
	return info, nil
}

// Android 依赖解析的时候传的数据
type AndroidParseExtra struct {
	CallBackUrl string            `json:"callBackUrl" form:"callBackUrl"`
	Extra       map[string]string `json:"extra" form:"extra"`
}

func ParseAndroidDeps(ctx context.Context, req *dependencyparse.ParseAndroidDepsRequest) (r *dependencyparse.ParseAndroidDepsResponse, err error) {

	callBackUrl := "https://bits.bytedance.net/openapi/v1/dependency_parse/android_deps_call_back"
	//extraData.CallBackUrl = "http://10.224.4.28:6789/inner_mpaas/v1/androidDepsCallBack"

	dict := req.Extra
	if len(dict) == 0 {
		dict = make(map[string]string)
	}
	dict["call_back"] = req.CallBack

	extra := AndroidParseExtra{
		callBackUrl,
		dict,
	}

	err = ParseAndroidDepsCore(ctx, req.Git, req.Branch, req.CommitId, req.ModuleName, extra)
	if err != nil {
		logs.CtxDebug(ctx, "error %+v", err)
	}
	return
}

func ParseAndroidDepsCore(ctx context.Context, git string, branch string, commitId string, moduleName string, extraData AndroidParseExtra) error {

	queryBody := make(map[string]string)
	queryBody["git"] = git
	queryBody["branch"] = branch
	queryBody["commitId"] = commitId
	queryBody["moduleName"] = moduleName

	byteArr, err := fjson.ConfigCompatibleWithStandardLibrary.Marshal(extraData)
	if err != nil {
		logs.CtxDebug(ctx, "error %+v", err)
		return err
	}
	extraDataStr := string(byteArr)
	queryBody["bytebusInfo"] = extraDataStr

	queueId, err := jenkins.CloudBuildCIJenkinsClient.StartNewJob(constDefine.DOUYIN_ANDROID_DEPENDENCY_ANALYZE, queryBody)

	logs.CtxDebug(ctx, "Param: %v, queueId %d %v ", queryBody, queueId, err)
	return err
}

