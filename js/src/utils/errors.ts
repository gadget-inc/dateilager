export class ProjectAlreadyExistsError extends Error {
  constructor(msg: string) {
    super(msg);
  }
}
