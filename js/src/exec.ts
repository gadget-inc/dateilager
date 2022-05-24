import { context, propagation } from "@opentelemetry/api";
import type { ExecaReturnValue } from "execa";
import execa from "execa";
import fse from "fs-extra";
import pMemoize from "p-memoize";
import type { Logger } from "pino";
import readline from "readline";
import { traced } from "./telemetry";

const DL_UPDATE_TIMEOUT = 10000;
const DL_REBUILD_TIMEOUT = 8000;

/**
 * A version of the DateiLager client that uses the compiled binary client instead of the Javascript one.
 *
 * Useful for working directly with a real filesystem instead of in memory objects.
 */
export class DateiLagerBinaryClient {
  logger: Logger;
  server: string;
  memoizedTokenFn: () => Promise<string>;
  tracing: boolean;

  constructor(logger: Logger, host: string, port: number, tokenFn: () => Promise<string>, tracing: boolean) {
    this.logger = logger;
    this.server = `${host}:${port}`;
    this.memoizedTokenFn = pMemoize(tokenFn);
    this.tracing = tracing;
  }

  @traced("dateilager-binary-client.update", {
    record: (project, diff, directory) => ({ project, diff, directory }),
  })
  async update(project: bigint, directory: string): Promise<bigint | null> {
    const result = await this._call("update", project, directory, DL_UPDATE_TIMEOUT, ["-dir", directory]);

    if (result.stdout == "-1") {
      return null;
    }

    return BigInt(result.stdout);
  }

  @traced("dateilager-binary-client.rebuild", {
    record: (project, from, to, output) => ({ project, from, to, output }),
  })
  async rebuild(project: bigint, from: bigint, to: bigint | null, directory: string): Promise<bigint | null> {
    let args = ["-from", String(from), "-dir", directory];
    if (to) {
      args = args.concat(["-to", String(to)]);
    }

    await fse.mkdirp(directory);
    const result = await this._call("rebuild", project, directory, DL_REBUILD_TIMEOUT, args);

    if (result.stdout == "-1") {
      return null;
    }

    return BigInt(result.stdout);
  }

  async _call(method: string, project: bigint, cwd: string, timeout: number, args: string[]): Promise<ExecaReturnValue<string>> {
    const level = this.logger.level == "trace" ? "debug" : this.logger.level;
    const baseArgs = [method, "-project", String(project), "-server", this.server, "-encoding", "json", "-log", level];

    if (this.tracing) {
      const carrier = {};
      propagation.inject(context.active(), carrier);
      const otelContext = JSON.stringify(carrier);

      baseArgs.push("-tracing", "-otel-context", otelContext);
    }

    const subprocess = execa("dateilager-client", baseArgs.concat(args), {
      cwd: cwd,
      timeout: timeout,
      env: { DL_TOKEN: await this.memoizedTokenFn() },
    });

    readline.createInterface(subprocess.stderr!).on("line", (line) => {
      try {
        const body = JSON.parse(line);
        const { level, ts, msg, ...rest } = body;

        // @ts-ignore
        this.logger[level](rest, msg);
      } catch (err: any) {
        this.logger.warn({ error: err.message }, line);
      }
    });

    return await subprocess;
  }
}
