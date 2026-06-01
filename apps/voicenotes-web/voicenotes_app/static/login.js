const form = document.getElementById("login-form");
const errorBox = document.getElementById("login-error");

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  errorBox.hidden = true;
  const payload = {
    email: document.getElementById("email").value,
    password: document.getElementById("password").value,
  };
  const response = await fetch("/api/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!response.ok) {
    errorBox.textContent = "Sign in failed";
    errorBox.hidden = false;
    return;
  }
  window.location.href = "/";
});

