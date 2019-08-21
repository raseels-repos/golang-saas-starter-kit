# SaaS Starter Kit

Copyright 2019, Geeks Accelerator 
twins@geeksaccelerator.com


## Description
_text-translator_ is a set of two tools: `extractor` and `translator`. Their goal is assisting in 
automatically generating resources to add support for internazionalied go-templates.

Theses tools aren't required for the build pipeline, but it might help in saving some boilerplate 
if you have many go-templates which harcoded messages in English.

In order to have a feeling of how to use these tools, see the next section which goes through a 
complete use-case for them.

## Usage
To understand how to use them, let's consider the `signup-step1.gohtml` from `web-app` which you 
can find in `cmd/web-app/content/`.
This template has hardcoded text in English. 

If you visit `http://localhost:3000/signup`:
![ImageSignup1](https://i.ibb.co/wdnFpNt/Screenshot-from-2019-08-20-17-32-21.png)

If you try to specify a custom `locale` with `http://localhost:3000/signup?locale=fr` or `http://localhost:3000/signup?locale=es` you will see the same webpage since English text is harcoded in the template.

Lets start by using the `extractor` tool to save us some time 
extracting harcoded texts to their corresponding `en` .json files 
which will be used by `universal-translator`.

Go to `tools/text-translator/extractor` and run:
```
go run main.go -i ../../../../cmd/web-app/templates/content/signup-step1.gohtml -o ../../../../cmd/web-app/templates/content/translations
```
This command takes the `signup-step1.gohtml` template file and generates a `universal-translator` file `cmd/web-app-templates/content/translations/en/signup-step1.json` with the extracted english texts. 

Now we should use the generated place holders in the `.gohtml` file. Currently this should be done manually, since the go-templates 
files aren't pure html (the underlying parser of the tool is `net/html`). This makes automatic replacement of found texts 
a bit harder since the files aren't 100% valid.

You can look at this particular example; the original and manually transformed template in `tools/cmd/extractor/.example.original.signup-step1.gohtml` and `tools/cmd/extractor/.example.transformed.signup-step1.gohtml`.

Since now the `.gohtml` uses the `universal-translator` through the `{{ $.trans.T <placeholder> }}` action, we should add proper support for other languages. For this, we'll leverage the `translator` tool to generate json files for other locales using the now existing `en` texts.

Now move to `tool/text-translator/cmd/translator` and run:
```
go run main.go -i ../../../../cmd/web-app/templates/content/translations/en/signup-step1.json -o ../../../../cmd/web-app/templates/content/translations -t fr,zh
```
The `extractor` will read the `en/signup/step1.json` and use `AWS Translator` to generate proper `.json` files for `fr` and `zh` locales. Remember that you should properly have configured env variables or the config folder with AWS credentials (`AWS_ACCESS_KEY`, `AWS_SECRET_ACCESS_KEY`, and region `AWS_DEFAULT_REGION`).

Now if you enter `http://localhost:3000?signup?locale=fr`, you'd see:
![ImageSignupFr](https://i.ibb.co/TrnX2q8/Screenshot-from-2019-08-20-21-09-12.png)

And `http://localhost:3000?signup?locale=zh`:
![ImageSignupZh](https://i.ibb.co/ZY0gnTj/Screenshot-from-2019-08-20-21-11-12.png)

Notice an important point: In this example there're some fields such 
as `Zipcode` and `Region` which didn't get extracted by `extractor`. In this case is because the HTML for these fields is generated dynamically with javascript, so it gets missed. This is one example of the border cases you should pay attention to.

## Caveats
These tools are still in its infancy and they have a lot of room for improvement. While they might help aliviating a lot of word with extracting, replacing and translating tasks, its results aren't flawless. In these three steps there may be unwanted outputs:
* all harcoded go-template texts might not be extracted
* some extracted texts might have details to fine-tune manually
* even if `AWS Translator` service is quite good, it may not be perfect

It's highly recommendable that you quickly look through the results and finish 
perfecting the result. Using `git` to properly see what changed in the `.gohtml` file, and verifying it was appropiate, would be a good advice to have in mind.

## Contribute!
We're open for contributions to improve these tools and make them better!