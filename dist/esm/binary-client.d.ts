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
     * @default "dateilager-client"
     */
    command?: string;
    /**
     * The default number of milliseconds to wait before terminating any command.
     * @default 0 No timeout.
     */
    timeout?: number | {
        /**
         * The default number of milliseconds to wait before terminating the update command.
         * @default 0 No timeout.
         */
        update?: number;
        /**
         * The default number of milliseconds to wait before terminating the rebuild command.
         * @default 0 No timeout.
         */
        rebuild?: number;
        /**
         * The default number of milliseconds to wait before terminating the gc command.
         * @default 0 No timeout.
         */
        gc?: number;
    };
    /**
     * Whether the dateilager binary client should enable tracing.
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
    /**
     * The path to the file where the profile will be written.
     */
    profile?: string;
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
export declare class DateiLagerBinaryClient {
    /**
     * @param options An object with {@link DateiLagerBinaryClientOptions options}.
     */
    constructor(options: DateiLagerBinaryClientOptions);
    /**
     * Update objects in a project based on the differences in a local directory.
     * @param project          The id of the project.
     * @param directory        The path of the directory to send updates from.
     * @param options          Object of options.
     * @param options.timeout  Number of milliseconds to wait before terminating the process.
     * @param options.subpaths The paths to include when updating the FS.
     * @returns                The latest project version or `null` if something went wrong.
     */
    update(project: bigint, directory: string, options?: {
        timeout?: number;
        subpaths?: string[];
    }): Promise<bigint | null>;
    /**
     * Rebuild the local filesystem.
     * @param project              The id of the project.
     * @param to                   The version of the project to rebuild the filesystem to.
     * @param directory            The path of the directory to rebuild the filesystem at.
     * @param options              Object of options.
     * @param options.timeout      Number of milliseconds to wait before terminating the process.
     * @param options.ignores      The paths to ignore when rebuilding the FS.
     * @param options.subpaths     The paths to include when rebuilding the FS.
     * @param options.summarize    Should produce the summary file after rebuilding.
     * @param options.cacheDir     Path where the cache directory is mounted.
     * @param options.matchInclude Set fileMatch to true if the written files are matched by this glob pattern
     * @param options.matchExclude Set fileMatch to false if the written files are matched by this glob pattern
     * @returns                    The latest project version or `null` if something went wrong.
     */
    rebuild(project: bigint, to: bigint | null, directory: string, options?: {
        timeout?: number;
        ignores?: string[];
        subpaths?: string[];
        summarize?: boolean;
        cacheDir?: string;
        matchInclude?: string;
        matchExclude?: string;
    }): Promise<RebuildResult>;
    /**
     * @param sample          sample size of cleanup
     * @param keep            The amount of records to keep
     * @param from            Where to start cleanup
     * @param options         dict options passed
     * @param options.timeout timeout limit for the request
     * @returns               The number of records garbage collected
     */
    gcRandomProjects(sample: number, keep: number, from?: number, options?: {
        timeout?: number;
    }): Promise<GCResult>;
    /**
     *
     * @param project         The selected project to cleanup
     * @param keep            The amount of records to keep
     * @param from            Where to start cleanup
     * @param options         dict options passed
     * @param options.timeout timeout limit for the request
     * @returns               The number of records garbage collected
     */
    gcProject(project: number, keep: number, from?: number, options?: {
        timeout?: number;
    }): Promise<GCResult>;
    /**
     *
     * @param sample          sample size of cleanup
     * @param options         dict options passed
     * @param options.timeout timeout limit for the request
     * @returns               The number of records garbage collected
     */
    gcContents(sample: number, options?: {
        timeout?: number;
    }): Promise<GCResult>;
}
