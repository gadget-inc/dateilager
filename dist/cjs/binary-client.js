"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.DateiLagerBinaryClient = void 0;
const api_1 = require("@opentelemetry/api");
const execa_1 = __importDefault(require("execa"));
const promises_1 = __importDefault(require("fs/promises"));
const readline_1 = __importDefault(require("readline"));
const telemetry_1 = require("./internal/telemetry");
/**
 * A version of the DateiLager client that uses the compiled binary client instead of the Javascript one.
 *
 * Useful for working directly with a real filesystem instead of in memory objects.
 */
class DateiLagerBinaryClient {
    /**
     * @param options An object with {@link DateiLagerBinaryClientOptions options}.
     */
    constructor(options) {
        /** @internal */
        Object.defineProperty(this, "_options", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: void 0
        });
        this._options = {
            host: options.server.host,
            port: options.server.port ?? 5051,
            headlessHost: options.server.headlessHost,
            token: typeof options.token === "string" ? () => Promise.resolve(options.token) : options.token,
            command: options.command ?? "dateilager-client",
            timeout: typeof options.timeout === "number"
                ? {
                    update: options.timeout,
                    rebuild: options.timeout,
                    gc: options.timeout,
                }
                : {
                    update: 0,
                    rebuild: 0,
                    gc: 0,
                    ...options.timeout,
                },
            tracing: options.tracing ?? false,
            logger: options.logger,
        };
    }
    /**
     * Update objects in a project based on the differences in a local directory.
     * @param project         The id of the project.
     * @param directory       The path of the directory to send updates from.
     * @param options         Object of options.
     * @param options.timeout Number of milliseconds to wait before terminating the process.
     * @returns               The latest project version or `null` if something went wrong.
     */
    async update(project, directory, options) {
        return await (0, telemetry_1.trace)("dateilager-binary-client.update", {
            attributes: {
                "dl.project": String(project),
                "dl.directory": directory,
            },
        }, async () => {
            const args = ["--dir", String(directory), "--project", String(project)];
            const result = await this._call("update", args, directory, options);
            if (result.stdout == "-1") {
                return null;
            }
            return BigInt(result.stdout);
        });
    }
    /**
     * Rebuild the local filesystem.
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
     * @returns                    The latest project version or `null` if something went wrong.
     */
    async rebuild(project, to, directory, options) {
        return await (0, telemetry_1.trace)("dateilager-binary-client.rebuild", {
            attributes: {
                "dl.project": String(project),
                "dl.to": String(to),
                "dl.directory": directory,
            },
        }, async () => {
            await promises_1.default.mkdir(directory, { recursive: true });
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
            const parsed = JSON.parse(result.stdout);
            return { version: BigInt(parsed.version), count: parsed.count, fileMatch: parsed.fileMatch };
        });
    }
    /**
     * @param sample          sample size of cleanup
     * @param keep            The amount of records to keep
     * @param from            Where to start cleanup
     * @param options         dict options passed
     * @param options.timeout timeout limit for the request
     */
    async gcRandomProjects(sample, keep, from, options) {
        return await (0, telemetry_1.trace)("dateilager-binary-client.gc", {
            attributes: {
                "db.mode": "random-projects",
                "dl.keep": String(keep),
                "dl.from": String(from),
                "dl.sample": String(sample),
            },
        }, async () => {
            const args = ["--mode", "random-projects", "--keep", String(keep), "--sample", String(sample)];
            if (from)
                args.push("--from", String(from));
            const result = await this._call("gc", args, undefined, options);
            const parsed = JSON.parse(result.stdout);
            return { count: parsed.count };
        });
    }
    /**
     *
     * @param project         The selected project to cleanup
     * @param keep            The amount of records to keep
     * @param from            Where to start cleanup
     * @param options         dict options passed
     * @param options.timeout timeout limit for the request
     */
    async gcProject(project, keep, from, options) {
        return await (0, telemetry_1.trace)("dateilager-binary-client.gc", {
            attributes: {
                "db.mode": "project",
                "dl.keep": String(keep),
                "dl.from": String(from),
            },
        }, async () => {
            const args = ["--mode", "project", "--keep", String(keep), "--project", String(project)];
            if (from)
                args.push("--from", String(from));
            const result = await this._call("gc", args, undefined, options);
            const parsed = JSON.parse(result.stdout);
            return { count: parsed.count };
        });
    }
    /**
     *
     * @param sample          sample size of cleanup
     * @param options         dict options passed
     * @param options.timeout timeout limit for the request
     */
    async gcContents(sample, options) {
        return await (0, telemetry_1.trace)("dateilager-binary-client.gc", {
            attributes: {
                "db.mode": "contents",
                "dl.sample": String(sample),
            },
        }, async () => {
            const args = ["--mode", "contents", "--sample", String(sample)];
            const result = await this._call("gc", args, undefined, options);
            const parsed = JSON.parse(result.stdout);
            return { count: parsed.count };
        });
    }
    /** @internal */
    async _call(method, args, cwd, options) {
        const baseArgs = [method, "--host", this._options.host, "--port", String(this._options.port), "--log-encoding", "json"];
        if (this._options.logger) {
            baseArgs.push("--log-level", this._options.logger.level);
        }
        if (this._options.headlessHost) {
            baseArgs.push("--headless-host", this._options.headlessHost);
        }
        const timeout = options?.timeout ?? this._options.timeout[method];
        baseArgs.push("--timeout", String(timeout));
        if (this._options.tracing) {
            const carrier = {};
            api_1.propagation.inject(api_1.context.active(), carrier);
            const otelContext = JSON.stringify(carrier);
            baseArgs.push("--tracing", "--otel-context", otelContext);
        }
        const subprocess = (0, execa_1.default)(this._options.command, baseArgs.concat(args), {
            cwd,
            cleanup: false, // don't terminate this subprocess process eagerly when the parent process is terminated, which is execa's default behaviour. we use graceful shutdown gadget-side to give running operations a chance to complete, and we don't want to terminate them prematurely
            env: { DL_TOKEN: await this._options.token() },
        });
        if (this._options.logger && subprocess.stderr) {
            readline_1.default.createInterface(subprocess.stderr).on("line", (line) => {
                try {
                    // we purposefully extract and ignore the `ts` field
                    // eslint-disable-next-line @typescript-eslint/no-unused-vars
                    const { level, msg, ts, ...fields } = JSON.parse(line);
                    this._options.logger?.log(level, msg, fields);
                }
                catch (error) {
                    this._options.logger?.log("warn", "failed to log line", { error, line });
                }
            });
        }
        return subprocess;
    }
}
exports.DateiLagerBinaryClient = DateiLagerBinaryClient;
//# sourceMappingURL=binary-client.js.map