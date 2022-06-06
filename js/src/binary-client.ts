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
  server:
    | string
    | {
        /**
         * The host of the dateilager server.
         */
        host: string;

        /**
         * The port of the dateilager server.
         */
        port: number;
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
 * A version of the DateiLager client that uses the compiled binary client instead of the Javascript one.
 *
 * Useful for working directly with a real filesystem instead of in memory objects.
 */
export class DateiLagerBinaryClient {
  /** @internal */
  private readonly _options;

  /**
   * @param  options An object with {@link DateiLagerBinaryClientOptions options}.
   */
  public constructor(options: DateiLagerBinaryClientOptions) {
    this._options = {
      server: typeof options.server === "string" ? options.server : `${options.server.host}:${options.server.port}`,
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
   * @param    project         The id of the project.
   * @param    directory       The path of the directory to send updates from.
   * @param    options         Object of options.
   * @param    options.timeout Number of milliseconds to wait before terminating the process.
   * @returns                  The latest project version or `null` if something went wrong.
   */
  public async update(project: bigint, directory: string, options?: { timeout: number }): Promise<bigint | null> {
    return await trace(
      "dateilager-binary-client.update",
      {
        attributes: {
          "dl.project": String(project),
          "dl.directory": directory,
        },
      },
      async () => {
        const result = await this._call("update", project, directory, ["-dir", directory], options);

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
   * @param    project         The id of the project.
   * @param    to              The version of the project to rebuild the filesystem to.
   * @param    directory       The path of the directory to rebuild the filesystem at.
   * @param    options         Object of options.
   * @param    options.timeout Number of milliseconds to wait before terminating the process.
   * @returns                  The latest project version or `null` if something went wrong.
   */
  public async rebuild(project: bigint, to: bigint | null, directory: string, options?: { timeout: number }): Promise<bigint | null> {
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

        const args = ["-dir", directory];
        if (to) {
          args.push("-to", String(to));
        }

        const result = await this._call("rebuild", project, directory, args, options);
        if (result.stdout == "-1") {
          return null;
        }

        return BigInt(result.stdout);
      }
    );
  }

  /** @internal */
  private async _call(
    method: "update" | "rebuild",
    project: bigint,
    cwd: string,
    args: string[],
    options?: { timeout: number }
  ): Promise<ExecaReturnValue> {
    const baseArgs = [method, "-project", String(project), "-server", this._options.server, "-encoding", "json"];

    if (this._options.logger) {
      baseArgs.push("-log", this._options.logger.level);
    }

    if (this._options.tracing) {
      const carrier = {};
      propagation.inject(context.active(), carrier);
      const otelContext = JSON.stringify(carrier);

      baseArgs.push("-tracing", "-otel-context", otelContext);
    }

    const subprocess = execa(this._options.command, baseArgs.concat(args), {
      cwd,
      timeout: options?.timeout ?? this._options.timeout[method],
      env: { DL_TOKEN: await this._options.token() },
    });

    if (this._options.logger && subprocess.stderr) {
      readline.createInterface(subprocess.stderr).on("line", (line) => {
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
          this._options.logger?.log("warn", "failed to log line", { error, line });
        }
      });
    }

    return subprocess;
  }
}
