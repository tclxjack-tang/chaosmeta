/*
 * Copyright 2022-2023 Chaos Meta Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package mem

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/traas-stack/chaosmeta/chaosmetad/pkg/injector"
	"github.com/traas-stack/chaosmeta/chaosmetad/pkg/log"
	"github.com/traas-stack/chaosmeta/chaosmetad/pkg/utils"
	"github.com/traas-stack/chaosmeta/chaosmetad/pkg/utils/cmdexec"
	"github.com/traas-stack/chaosmeta/chaosmetad/pkg/utils/namespace"
	"github.com/traas-stack/chaosmeta/chaosmetad/pkg/utils/process"
)

func init() {
	injector.Register(TargetMem, FaultMemOOM, func() injector.IInjector { return &OOMInjector{} })
}

type OOMInjector struct {
	injector.BaseInjector
	Args    OOMArgs
	Runtime OOMRuntime
}

type OOMArgs struct {
	Mode string `json:"mode,omitempty"`
}

type OOMRuntime struct {
	//Pid int `json:"pid,omitempty"`
}

func (i *OOMInjector) GetArgs() interface{} {
	return &i.Args
}

func (i *OOMInjector) GetRuntime() interface{} {
	return &i.Runtime
}

func (i *OOMInjector) SetDefault() {
	i.BaseInjector.SetDefault()

	if i.Args.Mode == "" {
		if i.Info.ContainerId != "" {
			i.Args.Mode = ModeRam
		} else {
			i.Args.Mode = ModeCache
		}
	}
}

func (i *OOMInjector) SetOption(cmd *cobra.Command) {
	// i.BaseInjector.SetOption(cmd)
	cmd.Flags().StringVarP(&i.Args.Mode, "mode", "m", "", fmt.Sprintf("mem fill mode, support: %s、%s（default %s）", ModeRam, ModeCache, ModeCache))
}

func (i *OOMInjector) getCmdExecutor(method, args string) *cmdexec.CmdExecutor {
	return &cmdexec.CmdExecutor{
		ContainerId:      i.Info.ContainerId,
		ContainerRuntime: i.Info.ContainerRuntime,
		ContainerNs:      []string{namespace.MNT, namespace.PID},
		ToolKey:          MemExec,
		Method:           method,
		Fault:            FaultMemFill,
		Args:             args,
	}
}

func (i *OOMInjector) Validator(ctx context.Context) error {
	if err := i.BaseInjector.Validator(ctx); err != nil {
		return err
	}

	if i.Args.Mode != ModeCache && i.Args.Mode != ModeRam {
		return fmt.Errorf("\"mode\" is not support: %s, only support: %s、%s", i.Args.Mode, ModeCache, ModeRam)
	}

	if i.Args.Mode == ModeCache {
		if i.Info.ContainerId != "" {
			return fmt.Errorf("not support mode \"cache\" in container")
		}

		return i.getCmdExecutor(utils.MethodValidator, "").ExecTool(ctx)
	}

	return nil
}

func getOOMDir(uid string) string {
	return fmt.Sprintf("%s%s", OOMDir, uid)
}

func (i *OOMInjector) Inject(ctx context.Context) error {
	logger := log.GetLogger(ctx)
	if i.Args.Mode == ModeRam {
		var timeout int64
		if i.Info.Timeout != "" {
			timeout, _ = utils.GetTimeSecond(i.Info.Timeout)
		}

		toolPath := utils.GetToolPath(MemFillKey)
		args := fmt.Sprintf("'%s' %d %d '%s' %d", i.Info.Uid, -999, PercentOOM, "", timeout)

		if i.Info.ContainerRuntime != "" {
			localPath := toolPath
			toolPath = utils.GetContainerPath(MemFillKey)
			if err := cmdexec.CpContainerFile(ctx, i.Info.ContainerRuntime, i.Info.ContainerId, localPath, toolPath); err != nil {
				return fmt.Errorf("container cp from [%s] to [%s] error: %s", localPath, toolPath, err.Error())
			}
		}

		cmd := fmt.Sprintf("%s %s", toolPath, args)
		if err := i.getCmdExecutor("", "").StartCmdAndWait(ctx, cmd); err != nil {
			if err := i.Recover(ctx); err != nil {
				logger.Warnf("undo error: %s", err.Error())
			}

			return err
		}
	} else {
		return i.getCmdExecutor(utils.MethodInject, fmt.Sprintf("%d '%s' '%s' '%s'", PercentOOM, "", getOOMDir(i.Info.Uid), TmpFsFile)).ExecTool(ctx)
	}

	return nil
}

func (i *OOMInjector) Recover(ctx context.Context) error {
	if i.BaseInjector.Recover(ctx) == nil {
		return nil
	}

	if i.Args.Mode == ModeCache {
		return i.getCmdExecutor(utils.MethodRecover, getOOMDir(i.Info.Uid)).ExecTool(ctx)
	} else {
		return process.CheckExistAndKillByKey(ctx, fmt.Sprintf("%s %s", MemFillKey, i.Info.Uid))
	}
}
