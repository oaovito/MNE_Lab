// Asset imports handled by the bundler.
declare module '*.css';
declare module '*.svg' {
  const text: string;
  export default text;
}
declare module '*.woff2' {
  const url: string;
  export default url;
}
