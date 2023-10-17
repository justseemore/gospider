# The creation database will be archived due to the following reasons. Core functions, please go to [requests](https://github.com/gospider007/requests)
* The address of the library creation module is gitee.com, which does not match github.com
* The code for creating the repository is too cumbersome, and the repository is composed of multiple independent modules. Next, these modules will be decomposed to become relatively independent modules in the future
* The core function of Gospider is the request library, which is limited by the official code's transport implementation, resulting in many unresolved issues that will be resolved in the new warehouse

# Introduction
### Gospider is a powerful Golang web crawler that includes all the necessary libraries for transitioning from Python to Golang. It provides a fast and seamless transition for Python web crawlers to Golang.
---
#### Module documentation can be found at the following link!!!
#### Module documentation can be found at the following link!!!
#### Module documentation can be found at the following link!!!
---
[Request Library](https://github.com/gospider007/requests): JA3 and HTTP/2 fingerprinting. Websocket, SSE, HTTP, and HTTPS protocols.
# Dependencies
```
go1.21 (Do not use a version lower than this)
```
# Installation (Do not fetch the package from GitHub, choose either Gitee or GitHub for the go package path. Fetching from GitHub will cause path issues.)
```
go get -u gitee.com/baixudong/gospider
```
# For easy management, please submit bugs on GitHub
```
https://github.com/baixudong007/gospider
```
# [Test Cases](../../tree/master/test)

# Recommended Libraries
|Library Name|Reason for Recommendation|
-|-
[curl_cffi](https://github.com/yifeikong/curl_cffi)|The best library for modifying JA3 fingerprints in Python.
[chromedp](https://github.com/chromedp/chromedp)|The best library for browser manipulation in Golang.