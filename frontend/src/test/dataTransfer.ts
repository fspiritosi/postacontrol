// DataTransfer mínimo: jsdom no trae uno.
export class FakeDataTransfer {
  data = new Map<string, string>();
  effectAllowed = "none";
  dropEffect = "none";
  get types() {
    return [...this.data.keys()];
  }
  setData(type: string, value: string) {
    this.data.set(type, value);
  }
  getData(type: string) {
    return this.data.get(type) ?? "";
  }
}
