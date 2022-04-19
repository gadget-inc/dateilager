import type { ExecaReturnValue } from "execa";
import execa from "execa";
import fse from "fs-extra";
import pMemoize from "p-memoize";
import path from "path";
import type { Logger } from "pino";
import readline from "readline";

const FSDIFF_TIMEOUT = 8000;
const FSDIFF_IGNORES = [".dl", ".dl/version", ".dl/sum.s2", ".dl/diff.s2"].join(",");
const DL_UPDATE_TIMEOUT = 10000;
const DL_REBUILD_TIMEOUT = 8000;

export type FsDiffOutputPaths = {
  diff: string;
  sum: string;
};

/**
 * A wrapper around the FsDiff binary, produces diffs for the binary DateiLager client.
 */
export class FsDiffClient {
  logger: Logger;

  constructor(logger: Logger) {
    this.logger = logger;
  }

  async diff(directory: string, output: string, sum?: string): Promise<FsDiffOutputPaths> {
    let args = ["-dir", directory, "-out", output, "-ignores", FSDIFF_IGNORES];
    if (sum) {
      args = args.concat(["-sum", sum]);
    }

    const subprocess = execa("fsdiff", args, { cwd: directory, timeout: FSDIFF_TIMEOUT });

    readline.createInterface(subprocess.stdout!).on("line", (line) => this.logger.info(line.trim()));
    readline.createInterface(subprocess.stderr!).on("line", (line) => this.logger.info(line.trim()));

    await subprocess;

    return {
      diff: path.join(output, "diff.s2"),
      sum: path.join(output, "sum.s2"),
    };
  }
}

/**
 * A version of the DateiLager client that uses the compiled binary client instead of the Javascript one.
 *
 * Used for uploading large diffs produced by FsDiff and for rebuilding FS state within a sandbox.
 */
export class DateiLagerBinaryClient {
  logger: Logger;
  server: string;
  memoizedTokenFn: () => Promise<string>;

  constructor(logger: Logger, host: string, port: number, tokenFn: () => Promise<string>) {
    this.logger = logger;
    this.server = `${host}:${port}`;
    this.memoizedTokenFn = pMemoize(tokenFn);
  }

  async update(project: bigint, diff: string, directory: string): Promise<bigint | null> {
    const result = await this._call("update", project, directory, DL_UPDATE_TIMEOUT, ["-diff", diff, "-directory", directory]);

    if (result.stdout == "-1") {
      return null;
    }

    return BigInt(result.stdout);
  }

  async rebuild(project: bigint, from: bigint, to: bigint | null, output: string): Promise<bigint | null> {
    let args = ["-from", String(from), "-output", output];
    if (to) {
      args = args.concat(["-to", String(to)]);
    }

    await fse.mkdirp(output);
    const result = await this._call("rebuild", project, output, DL_REBUILD_TIMEOUT, args);

    if (result.stdout == "-1") {
      return null;
    }

    const version = BigInt(result.stdout);
    await this.updateVersionFile(output, version);
    return version;
  }

  async updateVersionFile(output: string, version: bigint): Promise<void> {
    const dlDir = path.join(output, ".dl");

    await fse.mkdirp(dlDir);
    await fse.writeFile(path.join(dlDir, "version"), String(version));
  }

  async _call(method: string, project: bigint, cwd: string, timeout: number, args: string[]): Promise<ExecaReturnValue<string>> {
    const level = this.logger.level == "trace" ? "debug" : this.logger.level;
    const baseArgs = [method, "-project", String(project), "-server", this.server, "-encoding", "json", "-log", level];
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
