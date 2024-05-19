package pulumi

import (
	"errors"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"deploy/pkg/universe"
)

func Cmd() error {
	// trap on exit to clean up ~/.cache/shenanigans/tmp

	pulumi.Run(func(ctx *pulumi.Context) error {
		// Register the stack into outputs as well
		stack := ctx.Stack()

		_, err := universe.NewUniverse(ctx, stack)
		if err != nil {
			fmt.Printf("%v\n", err)
			_ = fmt.Errorf(err.Error())

			return err
		}

		// err = groups.CreateConfig(ctx, g)
		// if err != nil {
		// 	fmt.Printf("%v\n", err)
		// 	_ = fmt.Errorf(err.Error())

		// 	return err
		// }
		return nil
		return errors.New(fmt.Sprintf("main\n"))
	})
	return nil
}
