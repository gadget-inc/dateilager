import { context, propagation } from "@opentelemetry/api";
import type { ExecaReturnValue } from "execa";
import execa from "execa";
import fs from "fs/promises";
import readline from "readline";
import { trace } from "./internal/telemetry";

/**
 * Options for {@link DateiLagerBinaryClient}.
 */
export interface DateiLagerBinaryClientOptions {
  /**
   * The address of the dateilager server.
   */
  server: {
    /**
     * The host of the dateilager server.
     */
    host: string;

    /**
     * The port of the dateilager server.
     */
    port?: number;

    /**
     * The GRPC headless service hostname, used for load balancing across multiple servers.
     */
    headlessHost?: string;
  };

  /**
   * The token that will be sent as authorization metadata to the dateilager server.
   */
  token: string | (() => Promise<string>);

  /**
   * The path to the dateilager-client executable.
   *
   * @default "dateilager-client"
   */
  command?: string;

  /**
   * The default number of milliseconds to wait before terminating any command.
   *
   * @default 0 No timeout.
   */
  timeout?:
    | number
    | {
        /**
         * The default number of milliseconds to wait before terminating the update command.
         *
         * @default 0 No timeout.
         */
        update?: number;

        /**
         * The default number of milliseconds to wait before terminating the rebuild command.
         *
         * @default 0 No timeout.
         */
        rebuild?: number;

        /**
         * The default number of milliseconds to wait before terminating the gc command.
         *
         * @default 0 No timeout.
         */
        gc?: number;
      };

  /**
   * Whether the dateilager binary client should enable tracing.
   *
   * @default false
   */
  tracing?: boolean;

  /**
   * Logging configuration.
   */
  logger?: {
    /**
     * The log level to pass to the dateilager binary client.
     */
    level: "debug" | "info" | "warn" | "error";

    /**
     * The function that will be called when the dateilager binary client logs something.
     */
    log: (level: "debug" | "info" | "warn" | "error", msg: string, fields: Record<string, unknown>) => void;
  };
}

/**
 * The version of the filesystem after the call to rebuild.
 *
 * If a file pattern was included then patternDetected can be used to know if that filepattern was seen while rebuilding.
 */
export interface RebuildResult {
  /**
   * Filesystem version.
   */
  version: bigint;
  /**
   * Files updated by the rebuild operation.
   */
  count: number;
  /**
   * Whether or not the file match was detected.
   */
  fileMatch: boolean;
}

/**
 * The Result from calling the Dateilager Garbage Compiler
 */
export interface GCResult {
  /**
   * The number of records garbage collected
   */
  count: number;
}

/**
 * A version of the DateiLager client that uses the compiled binary client instead of the Javascript one.
 *
 * Useful for working directly with a real filesystem instead of in memory objects.
 */
export class DateiLagerBinaryClient {
  /** @internal */
  private readonly _options;

  /**
   * @param options An object with {@link DateiLagerBinaryClientOptions options}.
   */
  public constructor(options: DateiLagerBinaryClientOptions) {
    this._options = {
      host: options.server.host,
      port: options.server.port ?? 5051,
      headlessHost: options.server.headlessHost,
      token: typeof options.token === "string" ? () => Promise.resolve(options.token as string) : options.token,
      command: options.command ?? "dateilager-client",
      timeout:
        typeof options.timeout === "number"
          ? {
              update: options.timeout,
              rebuild: options.timeout,
            }
          : {
              update: 0,
              rebuild: 0,
              ...options.timeout,
            },
      tracing: options.tracing ?? false,
      logger: options.logger,
    };
  }

  /**
   * Update objects in a project based on the differences in a local directory.
   *
   * @param project         The id of the project.
   * @param directory       The path of the directory to send updates from.
   * @param options         Object of options.
   * @param options.timeout Number of milliseconds to wait before terminating the process.
   * @returns                 The latest project version or `null` if something went wrong.
   */
  public async update(project: bigint, directory: string, options?: { timeout?: number }): Promise<bigint | null> {
    return await trace(
      "dateilager-binary-client.update",
      {
        attributes: {
          "dl.project": String(project),
          "dl.directory": directory,
        },
      },
      async () => {
        const args = ["--dir", String(directory), "--project", String(project)];
        const result = await this._call("update", args, directory, options);

        if (result.stdout == "-1") {
          return null;
        }

        return BigInt(result.stdout);
      }
    );
  }

  /**
   * Rebuild the local filesystem.
   *
   * @param project              The id of the project.
   * @param to                   The version of the project to rebuild the filesystem to.
   * @param directory            The path of the directory to rebuild the filesystem at.
   * @param options              Object of options.
   * @param options.timeout      Number of milliseconds to wait before terminating the process.
   * @param options.ignores      The paths to ignore when rebuilding the FS.
   * @param options.summarize    Should produce the summary file after rebuilding.
   * @param options.cacheDir     Path where the cache directory is mounted.
   * @param options.matchInclude Set fileMatch to true if the written files are matched by this glob pattern
   * @param options.matchExclude Set fileMatch to false if the written files are matched by this glob pattern
   * @returns                      The latest project version or `null` if something went wrong.
   */
  public async rebuild(
    project: bigint,
    to: bigint | null,
    directory: string,
    options?: {
      timeout?: number;
      ignores?: string[];
      summarize?: boolean;
      cacheDir?: string;
      matchInclude?: string;
      matchExclude?: string;
    }
  ): Promise<RebuildResult> {
    return await trace(
      "dateilager-binary-client.rebuild",
      {
        attributes: {
          "dl.project": String(project),
          "dl.to": String(to),
          "dl.directory": directory,
        },
      },
      async () => {
        await fs.mkdir(directory, { recursive: true });

        const args = ["--dir", directory];
        if (to) {
          args.push("--to", String(to));
        }

        if (options?.ignores) {
          args.push("--ignores", options.ignores.join(","));
        }

        if (options?.summarize === false) {
          args.push("--summarize=false");
        }

        if (options?.cacheDir) {
          args.push(`--cachedir=${options.cacheDir}`);
        }

        if (options?.matchInclude) {
          args.push(`--matchinclude=${options.matchInclude}`);
        }

        if (options?.matchExclude) {
          args.push(`--matchexclude=${options.matchExclude}`);
        }

        args.push("--project", String(project), "--dir", directory);
        const result = await this._call("rebuild", args, directory, options);
        const parsed = JSON.parse(result.stdout) as { version: number; count: number; fileMatch: boolean };
        return { version: BigInt(parsed.version), count: parsed.count, fileMatch: parsed.fileMatch };
      }
    );
  }

  /**
   * @param sample          sample size of cleanup
   * @param keep            The amount of records to keep
   * @param from            Where to start cleanup
   * @param options         dict options passed
   * @param options.timeout timeout limit for the request
   */
  public async gcRandomProjects(sample: number, keep: number, from?: number, options?: { timeout?: number }): Promise<GCResult> {
    return await trace(
      "dateilager-binary-client.gc",
      {
        attributes: {
          "db.mode": "random-projects",
          "dl.keep": String(keep),
          "dl.from": String(from),
          "dl.sample": String(sample),
        },
      },
      async () => {
        const args = ["--mode", "random-projects", "--keep", String(keep), "--sample", String(sample)];

        if (from) args.push("--from", String(from));

        const result = await this._call("gc", args, undefined, options);
        const parsed = JSON.parse(result.stdout) as { count: number };
        return { count: parsed.count };
      }
    );
  }

  /**
   *
   * @param project         The selected project to cleanup
   * @param keep            The amount of records to keep
   * @param from            Where to start cleanup
   * @param options         dict options passed
   * @param options.timeout timeout limit for the request
   */
  public async gcProject(project: number, keep: number, from?: number, options?: { timeout?: number }): Promise<GCResult> {
    return await trace(
      "dateilager-binary-client.gc",
      {
        attributes: {
          "db.mode": "project",
          "dl.keep": String(keep),
          "dl.from": String(from),
        },
      },
      async () => {
        const args = ["--mode", "project", "--keep", String(keep), "--project", String(project)];

        if (from) args.push("--from", String(from));

        const result = await this._call("gc", args, undefined, options);
        const parsed = JSON.parse(result.stdout) as { count: number };
        return { count: parsed.count };
      }
    );
  }

  /**
   *
   * @param sample          sample size of cleanup
   * @param options         dict options passed
   * @param options.timeout timeout limit for the request
   */
  public async gcContents(sample: number, options?: { timeout?: number }): Promise<GCResult> {
    return await trace(
      "dateilager-binary-client.gc",
      {
        attributes: {
          "db.mode": "contents",
          "dl.sample": String(sample),
        },
      },
      async () => {
        const args = ["--mode", "contents", "--sample", String(sample)];

        const result = await this._call("gc", args, undefined, options);
        const parsed = JSON.parse(result.stdout) as { count: number };
        return { count: parsed.count };
      }
    );
  }

  /** @internal */
  private async _call(
    method: "update" | "rebuild" | "gc",
    args: string[],
    cwd?: string,
    options?: { timeout?: number }
  ): Promise<ExecaReturnValue> {
    const baseArgs = [method, "--host", this._options.host, "--port", String(this._options.port), "--log-encoding", "json"];

    if (this._options.logger) {
      baseArgs.push("--log-level", this._options.logger.level);
    }

    if (this._options.headlessHost) {
      baseArgs.push("--headless-host", this._options.headlessHost);
    }

    if (this._options.tracing) {
      const carrier = {};
      propagation.inject(context.active(), carrier);
      const otelContext = JSON.stringify(carrier);

      baseArgs.push("--tracing", "--otel-context", otelContext);
    }
    const subprocess = execa(this._options.command, baseArgs.concat(args), {
      cwd,
      timeout: options?.timeout ?? this._options.timeout[method],
      env: { DL_TOKEN: await this._options.token() },
    });

    if (this._options.logger && subprocess.stderr) {
      readline.createInterface(subprocess.stderr).on("line", (line) => {
        this.mikesStdErrHandler(line)
        // try {
        //   // we purposefully extract and ignore the `ts` field
        //   // eslint-disable-next-line @typescript-eslint/no-unused-vars
        //   const { level, msg, ts, ...fields } = JSON.parse(line) as {
        //     [key: string]: unknown;
        //     level: "debug" | "info" | "warn" | "error";
        //     msg: string;
        //     ts: string | number;
        //   };

        //   this._options.logger?.log(level, msg, fields);
        // } catch (error) {
        //   this._options.logger?.log("warn", "failed to log line", { error, line });
        // }
      });
    }

    return subprocess;
  }

  mikesStdErrHandler(line: string) {
    try {
      // we purposefully extract and ignore the `ts` field
      // eslint-disable-next-line @typescript-eslint/no-unused-vars
      const { level, msg, ts, ...fields } = JSON.parse(line) as {
        [key: string]: unknown;
        level: "debug" | "info" | "warn" | "error";
        msg: string;
        ts: string | number;
      };

      this._options.logger?.log(level, msg, fields);
    } catch (error) {
      const stdOutChopped = line.split(":");
      if (stdOutChopped.length > 3) {
        const errorName = stdOutChopped.slice(1, 2).join().trim();
        const stack = stdOutChopped.slice(2).join().trim();

        this._options.logger?.log("warn", "failed to log line", {
          error: {
            name: errorName,
            stack: stack,
          },
          line,
        });
      } else {
        this._options.logger?.log("warn", "failed to log line", { error, line });
      }
    }
  }
}
