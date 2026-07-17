// Package uecauth logs in to UEC web services and maintains an authenticated
// HTTP session. A Session authenticates on its first request, then refreshes
// itself with a fresh Cookie Jar before the first request made after the
// configured refresh interval.
package uecauth
