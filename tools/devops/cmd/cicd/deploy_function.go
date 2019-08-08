package cicd


// deployLambdaFuncRequest defines the details needed to deploy a function to AWS Lambda.
type deployLambdaFuncRequest struct {
	EnableLambdaVPC bool `validate:"omitempty"`

	FuncName                          string `validate:"required"`

}

