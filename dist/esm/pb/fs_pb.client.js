import { Fs } from "./fs_pb";
import { stackIntercept } from "@protobuf-ts/runtime-rpc";
/**
 * @generated from protobuf service pb.Fs
 */
export class FsClient {
    constructor(_transport) {
        Object.defineProperty(this, "_transport", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: _transport
        });
        Object.defineProperty(this, "typeName", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: Fs.typeName
        });
        Object.defineProperty(this, "methods", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: Fs.methods
        });
        Object.defineProperty(this, "options", {
            enumerable: true,
            configurable: true,
            writable: true,
            value: Fs.options
        });
    }
    /**
     * @generated from protobuf rpc: NewProject(pb.NewProjectRequest) returns (pb.NewProjectResponse);
     */
    newProject(input, options) {
        const method = this.methods[0], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: DeleteProject(pb.DeleteProjectRequest) returns (pb.DeleteProjectResponse);
     */
    deleteProject(input, options) {
        const method = this.methods[1], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: ListProjects(pb.ListProjectsRequest) returns (pb.ListProjectsResponse);
     */
    listProjects(input, options) {
        const method = this.methods[2], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: Get(pb.GetRequest) returns (stream pb.GetResponse);
     */
    get(input, options) {
        const method = this.methods[3], opt = this._transport.mergeOptions(options);
        return stackIntercept("serverStreaming", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: GetCompress(pb.GetCompressRequest) returns (stream pb.GetCompressResponse);
     */
    getCompress(input, options) {
        const method = this.methods[4], opt = this._transport.mergeOptions(options);
        return stackIntercept("serverStreaming", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: GetUnary(pb.GetUnaryRequest) returns (pb.GetUnaryResponse);
     */
    getUnary(input, options) {
        const method = this.methods[5], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: Update(stream pb.UpdateRequest) returns (pb.UpdateResponse);
     */
    update(options) {
        const method = this.methods[6], opt = this._transport.mergeOptions(options);
        return stackIntercept("clientStreaming", this._transport, method, opt);
    }
    /**
     * @generated from protobuf rpc: Rollback(pb.RollbackRequest) returns (pb.RollbackResponse);
     */
    rollback(input, options) {
        const method = this.methods[7], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: Inspect(pb.InspectRequest) returns (pb.InspectResponse);
     */
    inspect(input, options) {
        const method = this.methods[8], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: Snapshot(pb.SnapshotRequest) returns (pb.SnapshotResponse);
     */
    snapshot(input, options) {
        const method = this.methods[9], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: Reset(pb.ResetRequest) returns (pb.ResetResponse);
     */
    reset(input, options) {
        const method = this.methods[10], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: GcProject(pb.GcProjectRequest) returns (pb.GcProjectResponse);
     */
    gcProject(input, options) {
        const method = this.methods[11], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: GcRandomProjects(pb.GcRandomProjectsRequest) returns (pb.GcRandomProjectsResponse);
     */
    gcRandomProjects(input, options) {
        const method = this.methods[12], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: GcContents(pb.GcContentsRequest) returns (pb.GcContentsResponse);
     */
    gcContents(input, options) {
        const method = this.methods[13], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: CloneToProject(pb.CloneToProjectRequest) returns (pb.CloneToProjectResponse);
     */
    cloneToProject(input, options) {
        const method = this.methods[14], opt = this._transport.mergeOptions(options);
        return stackIntercept("unary", this._transport, method, opt, input);
    }
    /**
     * @generated from protobuf rpc: GetCache(pb.GetCacheRequest) returns (stream pb.GetCacheResponse);
     */
    getCache(input, options) {
        const method = this.methods[15], opt = this._transport.mergeOptions(options);
        return stackIntercept("serverStreaming", this._transport, method, opt, input);
    }
}
//# sourceMappingURL=fs_pb.client.js.map