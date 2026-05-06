const greet = (who) => `hello, ${who}!`;

console.log(greet("world"));
console.log("1 + 2 =", 1 + 2);

var p = new Promise((resolve) => setTimeout(resolve, 1)).then(() => {
  console.log("This is printed after a 1 ms delay");
});
console.log("This is printed immediately");
await p;
console.log("This is printed immediately after awaiting the promise");
try {
  var resp = await fetch("https://httpbin.org/get");
  var data = await resp.text();
  console.log("httpbin response:", data);
} catch (err) {
  console.error("Error fetching httpbin:", err);
}